package claude

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const fixtureClaudeToken = "fixture-claude-token"

func TestAnthropicRuntimeTranslatesResponsesBuffered(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received = captureAnthropicTranslatedRequest(t, request)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte("{\"id\":\"chat_1\",\"model\":\"claude-sonnet-4-6\",\"created\":1700000000,\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"hello\"}}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4}}"))
	}))
	defer server.Close()

	transport := newAnthropicTestTransport(t, server.URL+"/v1", server.Client())
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: anthropicMountPath + "/responses",
		Body: []byte("{\"model\":\"sonnet\",\"instructions\":\"system\",\"input\":\"hello\",\"stream\":false}"),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	assertAnthropicTranslatedRequest(t, received)
	assertAnthropicTranslatedResponse(t, response)
}

func captureAnthropicTranslatedRequest(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	if request.URL.Path != "/v1/chat/completions" {
		t.Fatalf("path = %q", request.URL.Path)
	}
	assertAnthropicOAuthHeaders(t, request, false)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	var received map[string]any
	if err := json.Unmarshal(body, &received); err != nil {
		t.Fatal(err)
	}
	return received
}

func assertAnthropicTranslatedRequest(t *testing.T, received map[string]any) {
	t.Helper()
	if received["model"] != "claude-sonnet-5-5" || received["stream"] != false {
		t.Fatalf("upstream request = %#v", received)
	}
	messages := received["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" || messages[1].(map[string]any)["content"] != "hello" {
		t.Fatalf("messages = %#v", messages)
	}
}

func assertAnthropicTranslatedResponse(t *testing.T, response *proxymodel.Response) {
	t.Helper()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var translated map[string]any
	if err := json.Unmarshal(content, &translated); err != nil {
		t.Fatal(err)
	}
	if translated["object"] != "response" || translated["id"] != "chat_1" || response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("translated response = %#v headers=%v", translated, response.Header)
	}
	usage := translated["usage"].(map[string]any)
	if usage["total_tokens"] != float64(7) {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestAnthropicRuntimeTranslatesResponsesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	transport := newAnthropicTestTransport(t, server.URL+"/v1", server.Client())
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: anthropicMountPath + "/v1/responses",
		Body: []byte(`{"input":"hello","stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "response.output_text.delta") || !strings.Contains(string(content), "response.completed") || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("SSE = %q headers=%v", content, response.Header)
	}
}

func TestAnthropicRuntimeChatAndMessagesPassthrough(t *testing.T) {
	var paths []string
	var versions []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		versions = append(versions, request.Header.Get("anthropic-version"))
		assertAnthropicOAuthHeaders(t, request, request.URL.Path == "/v1/messages")
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), `"model":"opus"`) {
			t.Fatalf("passthrough model was changed: %s", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	transport := newAnthropicTestTransport(t, server.URL+"/v1", server.Client())
	for _, path := range []string{"/chat/completions", "/messages"} {
		response, err := transport.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: anthropicMountPath + path,
			Body: []byte(`{"model":"opus","messages":[]}`),
		}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if strings.Join(paths, ",") != "/v1/chat/completions,/v1/messages" || versions[0] != "" || versions[1] != anthropicAPIVersion {
		t.Fatalf("paths=%v versions=%v", paths, versions)
	}
}

func TestAnthropicRuntimeModelsAndCompactAreLocal(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	transport := newAnthropicTestTransport(t, server.URL+"/v1", server.Client())
	response, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: anthropicMountPath + "/models"}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if called || !strings.Contains(string(content), "claude-sonnet-4-6") {
		t.Fatalf("models called upstream=%t body=%s", called, content)
	}
	single, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: anthropicMountPath + "/models/CLAUDE-SONNET-4-6"}, proxymodel.Account{})
	if err != nil || single.StatusCode != http.StatusOK {
		t.Fatalf("single model = %#v, err=%v", single, err)
	}
	single.Body.Close()
	compact, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: anthropicMountPath + "/responses/compact",
		Body: []byte(`{"model":"claude-sonnet-4-6","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"retain compact context"}]}]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	compactBody, _ := io.ReadAll(compact.Body)
	compact.Body.Close()
	if called || compact.StatusCode != http.StatusOK ||
		compact.Header.Get("X-Prodex-Compact-Mode") != "local-fallback" ||
		compact.Header.Get("X-Prodex-Compact-Provider") != "anthropic" ||
		compact.Header.Get("X-Prodex-Compact-Degraded") != "true" ||
		compact.Header.Get("X-Prodex-Compact-Reason") != "local-policy" ||
		!strings.Contains(string(compactBody), "retain compact context") {
		t.Fatalf("compact = called:%t status:%d headers:%v body:%s", called, compact.StatusCode, compact.Header, compactBody)
	}
}

func TestAnthropicRuntimeForwardsNonHopHeadersAndReplacesCallerAuth(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		got = request.Header.Clone()
		_, _ = writer.Write([]byte("{\"ok\":true}"))
	}))
	defer server.Close()
	transport := newAnthropicTestTransport(t, server.URL+"/v1", server.Client())
	headers := http.Header{
		"Authorization":                    {"Bearer caller-secret"},
		"Chatgpt-Account-Id":               {"caller-account"},
		"Connection":                       {"keep-alive, X-Local-Hop"},
		"X-Local-Hop":                      {"strip-me"},
		"X-Prodex-Internal-Request-Origin": {"strip-me"},
		"X-Codex-Turn-State":               {"turn-state"},
		"Session_Id":                       {"session-state"},
		"X-Custom":                         {"keep-me"},
		"User-Agent":                       {"codex-cli-test"},
	}
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: anthropicMountPath + "/chat/completions",
		Header: headers, Body: []byte("{\"model\":\"sonnet\",\"messages\":[]}"),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	for _, name := range []string{"Chatgpt-Account-Id", "Connection", "X-Local-Hop", "X-Prodex-Internal-Request-Origin"} {
		if got.Get(name) != "" {
			t.Fatalf("%s unexpectedly forwarded: %v", name, got)
		}
	}
	if got.Get("Authorization") != "Bearer "+fixtureClaudeToken ||
		got.Get("X-Codex-Turn-State") != "turn-state" ||
		got.Get("Session_Id") != "session-state" ||
		got.Get("X-Custom") != "keep-me" ||
		got.Get("User-Agent") != "codex-cli-test" {
		t.Fatalf("forwarded headers = %v", got)
	}
}

func TestAnthropicRuntimeRejectsUnsupportedAndUnsafeRoutes(t *testing.T) {
	transport := newAnthropicTestTransport(t, "https://api.anthropic.com/v1", http.DefaultClient)
	for _, path := range []string{
		anthropicMountPath + "/v2/responses",
		anthropicMountPath + "/v1.2/responses",
		anthropicMountPath + "/responses/",
		anthropicMountPath + "/embeddings",
	} {
		if _, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: path, Body: []byte(`{}`)}, proxymodel.Account{}); err == nil {
			t.Fatalf("route %q unexpectedly accepted", path)
		}
	}
	if _, err := newRuntimeTransport("file:///tmp/anthropic", RuntimeOAuth{accessToken: fixtureClaudeToken}, nil); err == nil {
		t.Fatal("unsafe Anthropic URL accepted")
	}
}

func newAnthropicTestTransport(t *testing.T, upstream string, client *http.Client) *RuntimeTransport {
	t.Helper()
	transport, err := newRuntimeTransport(upstream, RuntimeOAuth{accessToken: fixtureClaudeToken}, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transport.Close)
	return transport
}

func assertAnthropicOAuthHeaders(t *testing.T, request *http.Request, messages bool) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer "+fixtureClaudeToken || request.Header.Get("anthropic-beta") != anthropicOAuthBeta || request.Header.Get("Accept-Encoding") != "identity" {
		t.Fatalf("headers = %v", request.Header)
	}
	if messages && request.Header.Get("anthropic-version") != anthropicAPIVersion {
		t.Fatalf("messages headers = %v", request.Header)
	}
	if !messages && request.Header.Get("anthropic-version") != "" {
		t.Fatalf("chat headers unexpectedly contain Anthropic version: %v", request.Header)
	}
}

func TestRuntimeOAuthReadsManagedCredential(t *testing.T) {
	home := t.TempDir()
	content := `{"claudeAiOauth":{"accessToken":"fixture-claude-token","expiresAt":4102444800000}}`
	if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	auth, err := NewSource().RuntimeOAuth(context.Background(), home)
	if err != nil || auth.accessToken != fixtureClaudeToken {
		t.Fatalf("auth = %#v, err=%v", auth, err)
	}
}

func TestAnthropicAPIKeyHeadersMatchProdexNativeAndChatModes(t *testing.T) {
	type captured struct {
		path   string
		header http.Header
	}
	requests := make(chan captured, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- captured{path: request.URL.Path, header: request.Header.Clone()}
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v1/chat/completions" {
			_, _ = writer.Write([]byte(`{"id":"chat","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	transport, err := newRuntimeAPIKeyTransport(server.URL+"/v1", "fixture-api-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	for _, request := range []proxymodel.Request{
		{Method: http.MethodPost, Path: anthropicMountPath + "/responses", Body: []byte(`{"input":"hello"}`)},
		{Method: http.MethodPost, Path: anthropicMountPath + "/chat/completions", Body: []byte(`{"model":"claude-sonnet-4-6","messages":[]}`)},
		{Method: http.MethodPost, Path: anthropicMountPath + "/messages", Body: []byte(`{"model":"claude-sonnet-4-6","messages":[]}`)},
	} {
		response, err := transport.Execute(context.Background(), request, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	for index := 0; index < 3; index++ {
		got := <-requests
		assertAnthropicAPIKeyHeaders(t, got.path, got.header)
	}
}

func assertAnthropicAPIKeyHeaders(t *testing.T, path string, header http.Header) {
	t.Helper()
	if path == "/v1/messages" {
		if header.Get("x-api-key") != "fixture-api-key" || header.Get("Authorization") != "" ||
			header.Get("anthropic-beta") != "" || header.Get("anthropic-version") != anthropicAPIVersion {
			t.Fatalf("native Messages API-key headers = %v", header)
		}
		return
	}
	if header.Get("Authorization") != "Bearer fixture-api-key" || header.Get("x-api-key") != "" ||
		header.Get("anthropic-beta") != "" || header.Get("anthropic-version") != "" {
		t.Fatalf("chat-compatible API-key headers = %v", header)
	}
}
