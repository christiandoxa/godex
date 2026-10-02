package deepseek

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

func TestDeepSeekRouteSurfaceMatchesRegistry(t *testing.T) {
	accepted := []string{
		mountPath + "/responses",
		mountPath + "/v1/responses",
		mountPath + "/responses/compact",
		mountPath + "/chat/completions",
		mountPath + "/messages",
		mountPath + "/models",
		mountPath + "/models/deepseek-v4-pro",
	}
	for _, path := range accepted {
		if _, err := runtimeRoute(path); err != nil {
			t.Fatalf("route %q rejected: %v", path, err)
		}
	}
	for _, path := range []string{
		mountPath + "/v2/responses",
		mountPath + "/responses/",
		mountPath + "/v1//responses",
		mountPath + "/v1/%2e%2e/responses",
		mountPath + "/embeddings",
	} {
		if _, err := runtimeRoute(path); err == nil {
			t.Fatalf("unsupported route %q accepted", path)
		}
	}
}

func TestDeepSeekMessagesPathMatchesReferenceNormalization(t *testing.T) {
	for base, want := range map[string]string{
		"":              "/anthropic/v1/messages",
		"/v1":           "/anthropic/v1/messages",
		"/beta":         "/anthropic/v1/messages",
		"/anthropic":    "/anthropic/v1/messages",
		"/anthropic/v1": "/anthropic/v1/messages",
		"/tenant/v1":    "/tenant/anthropic/v1/messages",
	} {
		if got := deepSeekMessagesPath(base); got != want {
			t.Fatalf("messages path for %q = %q, want %q", base, got, want)
		}
	}
}

func TestDeepSeekResponsesTranslateFallbackAndHeaders(t *testing.T) {
	type captured struct {
		path  string
		auth  string
		model string
	}
	var requests []captured
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		model, _ := value["model"].(string)
		requests = append(requests, captured{path: request.URL.Path, auth: request.Header.Get("Authorization"), model: model})
		writer.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"model_not_supported"}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"chat_ok","model":"deepseek-v4-flash","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`))
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"model":"pro","input":"hello"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"object":"response"`) {
		t.Fatalf("translated response = status:%d body:%s", response.StatusCode, body)
	}
	if len(requests) != 2 || requests[0].model != "deepseek-v4-pro" || requests[1].model != "deepseek-v4-flash" {
		t.Fatalf("fallback requests = %#v", requests)
	}
	for _, request := range requests {
		if request.path != "/chat/completions" || request.auth != "Bearer fixture-key" {
			t.Fatalf("request = %#v", request)
		}
	}
}

func TestDeepSeekAdvancedResponsesTranslateBeforeUpstream(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		content, _ := io.ReadAll(request.Body)
		var body map[string]any
		if err := json.Unmarshal(content, &body); err != nil {
			t.Fatalf("upstream body = %s, err = %v", content, err)
		}
		bodies = append(bodies, body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chat_ok","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	for _, body := range []string{
		`{"input":"hello","reasoning":{"effort":"high"}}`,
		`{"input":"hello","response_format":{"type":"json_object"}}`,
	} {
		response, err := transport.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(body),
		}, proxymodel.Account{})
		if err != nil {
			t.Fatalf("advanced request %s error = %v", body, err)
		}
		response.Body.Close()
	}
	if len(bodies) != 2 {
		t.Fatalf("advanced requests reaching upstream = %d", len(bodies))
	}
	if bodies[0]["reasoning_effort"] != "high" {
		t.Fatalf("reasoning request = %#v", bodies[0])
	}
	thinking, ok := bodies[0]["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking request = %#v", bodies[0])
	}
	if format, ok := bodies[1]["response_format"].(map[string]any); !ok || format["type"] != "json_object" {
		t.Fatalf("JSON response format = %#v", bodies[1])
	}
	messages := bodies[1]["messages"].([]any)
	if len(messages) < 2 || messages[0].(map[string]any)["content"] != "Respond with valid JSON only." {
		t.Fatalf("JSON prompt messages = %#v", messages)
	}

}

func TestDeepSeekUnsupportedWebSearchFailsBeforeUpstream(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	_, err = transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"input":"hello","web_search_options":{}}`),
	}, proxymodel.Account{})
	if err == nil || !strings.Contains(err.Error(), "web_search_options") {
		t.Fatalf("web-search request error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("unsupported web-search request reached upstream %d time(s)", calls)
	}
}

func TestDeepSeekChatAndMessagesRemainPassthrough(t *testing.T) {
	type captured struct {
		path, authorization, apiKey, version, body string
	}
	requests := make(chan captured, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- captured{
			path: request.URL.Path, authorization: request.Header.Get("Authorization"),
			apiKey: request.Header.Get("x-api-key"), version: request.Header.Get("anthropic-version"), body: string(body),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL+"/v1", "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	chatBody := `{"model":"flash","messages":[{"role":"user","content":"chat"}]}`
	messagesBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"native"}]}`
	for path, body := range map[string]string{
		mountPath + "/chat/completions": chatBody,
		mountPath + "/messages":         messagesBody,
	} {
		response, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: path, Body: []byte(body)}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	for index := 0; index < 2; index++ {
		got := <-requests
		switch got.path {
		case "/v1/chat/completions":
			if got.authorization != "Bearer fixture-key" || got.apiKey != "" || got.version != "" || got.body != chatBody {
				t.Fatalf("chat passthrough = %#v", got)
			}
		case "/anthropic/v1/messages":
			if got.authorization != "" || got.apiKey != "fixture-key" || got.version != anthropicVersion || got.body != messagesBody {
				t.Fatalf("messages passthrough = %#v", got)
			}
		default:
			t.Fatalf("unexpected passthrough request = %#v", got)
		}
	}
}

func TestDeepSeekModelsAndCompactAreLocal(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	models, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: mountPath + "/models"}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	modelsBody, _ := io.ReadAll(models.Body)
	models.Body.Close()
	for _, model := range []string{"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-chat", "deepseek-reasoner"} {
		if !strings.Contains(string(modelsBody), model) {
			t.Fatalf("models body missing %q: %s", model, modelsBody)
		}
	}
	compact, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact",
		Body: []byte(`{"model":"deepseek-v4-pro","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"retain me"}]}]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	compactBody, _ := io.ReadAll(compact.Body)
	compact.Body.Close()
	if called || compact.Header.Get("X-Prodex-Compact-Provider") != "deepseek" || !strings.Contains(string(compactBody), "retain me") {
		t.Fatalf("local endpoints = called:%t headers:%v body:%s", called, compact.Header, compactBody)
	}
}

func TestDeepSeekBare429DoesNotAdvanceModel(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"too many requests"}}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(`{"model":"pro","input":"hello"}`)}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls != 1 || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("calls/status = %d / %d", calls, response.StatusCode)
	}
}
