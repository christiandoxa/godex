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

func TestRuntimeTransportRejectsNonResponsesAndUnsafeURL(t *testing.T) {
	for _, upstream := range []string{"file:///tmp/copilot", "https://user:secret@example.test", "https://example.test?token=secret"} {
		if _, err := NewRuntimeTransport(upstream, RuntimeAuth{apiKey: "runtime-fixture"}, nil); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("upstream %q error = %v", upstream, err)
		}
	}
	transport, err := NewRuntimeTransport("https://example.test", RuntimeAuth{apiKey: "runtime-fixture"}, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/prodex/chat/completions", Body: []byte(`{}`)}, proxymodel.Account{}); err == nil {
		t.Fatal("non-Responses route unexpectedly accepted")
	}
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
		if model == "gpt-5.3-codex" {
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
	if response.StatusCode != http.StatusOK || len(models) != 2 || models[0] != "gpt-5.3-codex" || models[1] != "gpt-5.1-codex" {
		t.Fatalf("response/models = %d / %#v", response.StatusCode, models)
	}
}

func TestRuntimeTransportStructured429FallsBackButBare429DoesNot(t *testing.T) {
	for _, test := range []struct {
		name     string
		body     string
		attempts int
	}{
		{"structured", `{"error":{"code":"rate_limit_exceeded"}}`, 2},
		{"bare", `{"error":{"message":"too many requests"}}`, 1},
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
		t.Fatalf("bare 429 status = %d", response.StatusCode)
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
