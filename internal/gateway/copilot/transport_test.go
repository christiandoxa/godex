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
