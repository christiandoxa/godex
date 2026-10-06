package copilot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestRuntimeTransportForwardsResponsesWithReferenceHeaders(t *testing.T) {
	var gotPath, gotQuery string
	var gotHeader http.Header
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotPath, gotQuery = request.URL.Path, request.URL.RawQuery
		gotHeader = request.Header.Clone()
		gotBody, _ = io.ReadAll(request.Body)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{apiKey: "runtime-fixture"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"model":"codex","input":[{"type":"message","role":"assistant","content":[{"type":"input_image","file_id":"file-1"}]},{"type":"compaction","encrypted_content":"keep"}],"reasoning":{"encrypted_content":"drop"}}`)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", RawQuery: "stream=true", Body: body,
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if gotPath != "/responses" || gotQuery != "stream=true" {
		t.Fatalf("upstream target = %q?%s", gotPath, gotQuery)
	}
	for name, want := range map[string]string{
		"Authorization": "Bearer runtime-fixture", "Content-Type": "application/json",
		"Accept-Encoding": "identity", "Accept": "text/event-stream, application/json",
		"Copilot-Integration-Id": runtimeIntegrationID, "Openai-Intent": "conversation-panel",
		"X-GitHub-Api-Version": runtimeAPIVersion, "X-Initiator": "agent",
		"User-Agent": copilotRuntimeUserAgent, "Copilot-Vision-Request": "true",
	} {
		if gotHeader.Get(name) != want {
			t.Fatalf("header %s = %q, want %q", name, gotHeader.Get(name), want)
		}
	}
	if !strings.HasPrefix(gotHeader.Get("X-Request-Id"), "godex-") {
		t.Fatalf("request id = %q", gotHeader.Get("X-Request-Id"))
	}
	var value map[string]any
	if err := json.Unmarshal(gotBody, &value); err != nil {
		t.Fatal(err)
	}
	if value["model"] != defaultRuntimeModel {
		t.Fatalf("model = %#v", value["model"])
	}
	if _, ok := value["reasoning"].(map[string]any)["encrypted_content"]; ok {
		t.Fatalf("encrypted reasoning remained: %#v", value["reasoning"])
	}
	input := value["input"].([]any)
	if input[1].(map[string]any)["encrypted_content"] != "keep" {
		t.Fatalf("compaction changed: %#v", input[1])
	}
}

func TestRuntimeTransportMapsCompactAndLegacyPaths(t *testing.T) {
	paths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths <- request.URL.Path
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL+"/base", RuntimeAuth{apiKey: "runtime-fixture"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/backend-api/prodex/responses/compact", "/backend-api/prodex/v1/responses"} {
		response, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: path, Body: []byte(`{"model":"gpt-5.3-codex"}`)}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
	if first, second := <-paths, <-paths; first != "/base/responses/compact" || second != "/base/responses" {
		t.Fatalf("paths = %q, %q", first, second)
	}
}

func TestCopilotRuntimeRouteMatchesProdexV1SurfaceExactly(t *testing.T) {
	for _, fixture := range []struct {
		path string
		want string
	}{
		{"/backend-api/prodex/responses", "/responses"},
		{"/backend-api/prodex/v1/responses", "/responses"},
		{"/backend-api/prodex/responses/compact", "/responses/compact"},
		{"/backend-api/prodex/v1/chat/completions", "/chat/completions"},
		{"/backend-api/prodex/v1/messages", "/messages"},
	} {
		route, err := copilotRuntimeRoute(fixture.path)
		if err != nil || route.kind != copilotRouteUpstream || route.upstreamPath != fixture.want {
			t.Fatalf("route %q = %#v, err=%v; want upstream %q", fixture.path, route, err, fixture.want)
		}
	}
	for _, path := range []string{
		"/backend-api/prodex/v2/responses",
		"/backend-api/prodex/v1.2/responses",
		"/backend-api/prodex/v1/responses/compact/",
		"/backend-api/prodex/v1//responses",
		"/backend-api/prodex/v1/%2e%2e/responses",
		"/backend-api/prodex/tenant/v1/responses",
	} {
		if _, err := copilotRuntimeRoute(path); err == nil {
			t.Fatalf("unsupported route %q unexpectedly accepted", path)
		}
	}
}

func TestRuntimeTransportRejectsUnsupportedRoutesAndUnsafeURL(t *testing.T) {
	for _, upstream := range []string{"file:///tmp/copilot", "https://user:secret@example.test", "https://example.test?token=<redacted>"} {
		if _, err := NewRuntimeTransport(upstream, RuntimeAuth{apiKey: "<redacted>"}, nil); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("upstream %q error = %v", upstream, err)
		}
	}
	transport, err := NewRuntimeTransport("https://example.test", RuntimeAuth{apiKey: "<redacted>"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/embeddings", Body: []byte(`{}`),
	}, proxymodel.Account{}); err == nil {
		t.Fatal("unsupported embeddings route unexpectedly accepted")
	}
}

func TestRuntimeTransportPassesThroughChatAndMessages(t *testing.T) {
	type upstreamRequest struct {
		path   string
		header http.Header
		body   []byte
	}
	requests := make(chan upstreamRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- upstreamRequest{path: request.URL.Path, header: request.Header.Clone(), body: body}
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{apiKey: "<redacted>"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	trace := http.Header{
		"Traceparent": {"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
		"Tracestate":  {"prodex=test"},
		"Baggage":     {"tenant_tier=premium"},
	}
	for _, request := range []proxymodel.Request{
		{
			Method: http.MethodPost, Path: "/backend-api/prodex/v1/chat/completions", Header: trace,
			Body: []byte(`{"model":"codex","messages":[{"role":"assistant","reasoning":{"encrypted_content":"drop"}}]}`),
		},
		{
			Method: http.MethodPost, Path: "/backend-api/prodex/messages",
			Body: []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}]}`),
		},
	} {
		response, err := transport.Execute(context.Background(), request, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
	chat := <-requests
	messages := <-requests
	if chat.path != "/chat/completions" || messages.path != "/messages" {
		t.Fatalf("upstream paths = %q / %q", chat.path, messages.path)
	}
	for name, want := range map[string]string{
		"Traceparent": trace.Get("Traceparent"), "Tracestate": trace.Get("Tracestate"), "Baggage": trace.Get("Baggage"),
	} {
		if chat.header.Get(name) != want {
			t.Fatalf("trace header %s = %q, want %q", name, chat.header.Get(name), want)
		}
	}
	var chatBody map[string]any
	if err := json.Unmarshal(chat.body, &chatBody); err != nil {
		t.Fatal(err)
	}
	if chatBody["model"] != defaultRuntimeModel {
		t.Fatalf("chat model = %#v", chatBody["model"])
	}
	reasoning := chatBody["messages"].([]any)[0].(map[string]any)["reasoning"].(map[string]any)
	if _, ok := reasoning["encrypted_content"]; ok {
		t.Fatalf("chat encrypted content remained: %#v", chatBody)
	}
}

func TestRuntimeTransportEmulatesModelsEndpointsLocally(t *testing.T) {
	upstreamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls++
		http.Error(writer, "models should be local", http.StatusInternalServerError)
	}))
	defer server.Close()
	dynamic := map[string]any{
		"id": "Dynamic-X", "object": "model", "owned_by": "github-copilot",
		"display_name": "Dynamic X", "context_window": uint64(321000),
	}
	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{
		apiKey: "<redacted>", modelCatalog: []map[string]any{dynamic},
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	list, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/prodex/v1/models",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer list.Body.Close()
	var listBody struct {
		Object string           `json:"object"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if list.StatusCode != http.StatusOK || listBody.Object != "list" || upstreamCalls != 0 {
		t.Fatalf("models list = status:%d object:%q upstream:%d", list.StatusCode, listBody.Object, upstreamCalls)
	}
	if !catalogHasModel(listBody.Data, defaultRuntimeModel) || !catalogHasModel(listBody.Data, "Dynamic-X") {
		t.Fatalf("models list missing static/dynamic entries: %#v", listBody.Data)
	}

	single, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/prodex/models/dynamic-x",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer single.Body.Close()
	var singleBody map[string]any
	if err := json.NewDecoder(single.Body).Decode(&singleBody); err != nil {
		t.Fatal(err)
	}
	if single.StatusCode != http.StatusOK || singleBody["id"] != "Dynamic-X" || upstreamCalls != 0 {
		t.Fatalf("single model = status:%d body:%#v upstream:%d", single.StatusCode, singleBody, upstreamCalls)
	}

	missing, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/prodex/models/ Dynamic-X ",
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound || upstreamCalls != 0 {
		t.Fatalf("trimmed model lookup unexpectedly matched: status=%d upstream=%d", missing.StatusCode, upstreamCalls)
	}
}

func catalogHasModel(models []map[string]any, id string) bool {
	for _, model := range models {
		if value, _ := model["id"].(string); strings.EqualFold(value, id) {
			return true
		}
	}
	return false
}

func TestRuntimeTransportFallsBackToNextModelBeforeCommit(t *testing.T) {
	var models []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]any
		content, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(content, &body); err != nil {
			t.Fatal(err)
		}
		model, _ := body["model"].(string)
		models = append(models, model)
		if model == "gpt-6-astra" {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"model_not_supported"}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"response-ok"}`))
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{apiKey: "runtime-fixture"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", Body: []byte(`{"model":"codex","input":[]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || len(models) != 2 || models[0] != "gpt-6-astra" || models[1] != "gpt-6.1-sol" {
		t.Fatalf("response/models = %d / %#v", response.StatusCode, models)
	}
}

func TestProdex04356RuntimeTransportGeneric429FallsBackBeforeCommit(t *testing.T) {
	for _, test := range []struct {
		name     string
		body     string
		attempts int
	}{
		{"structured", `{"error":{"code":"rate_limit_exceeded"}}`, 2},
		{"bare", `{"error":{"message":"too many requests"}}`, 1},
		{"model not supported", `{"error":{"code":"model_not_supported"}}`, 2},
		{"invalid request", `{"error":{"type":"invalid_request_error"}}`, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertRuntime429Attempts(t, test.body, test.attempts)
		})
	}
}

func assertRuntime429Attempts(t *testing.T, errorBody string, wantAttempts int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if calls == 1 {
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = writer.Write([]byte(errorBody))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"fallback-ok"}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{apiKey: "runtime-fixture"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", Body: []byte(`{"model":"codex","input":[]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls != wantAttempts {
		t.Fatalf("calls = %d, want %d", calls, wantAttempts)
	}
	if wantAttempts == 1 && response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("terminal 429 status = %d", response.StatusCode)
	}
}

func TestRuntimeTransportAuthFailureDoesNotFallbackModels(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"code":"invalid_api_key"}}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{apiKey: "runtime-fixture"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", Body: []byte(`{"model":"codex","input":[]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls != 1 || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("calls/status = %d / %d", calls, response.StatusCode)
	}
}

func TestRuntimeTransportPreservesBufferedNonRetryableErrorBody(t *testing.T) {
	const payload = `{"error":{"code":"invalid_request","message":"keep this body"}}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Upstream-Error", "fixture")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(payload))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, RuntimeAuth{apiKey: "runtime-fixture"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", Body: []byte(`{"model":"codex","input":[]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || response.Header.Get("X-Upstream-Error") != "fixture" || string(body) != payload {
		t.Fatalf("buffered response = %d / %q / %q", response.StatusCode, response.Header.Get("X-Upstream-Error"), body)
	}
}
