package gemini

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

func TestGeminiSemanticCompactUsesSelectedModelAndSummary(t *testing.T) {
	var paths []string
	var nativeBodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		var outgoing map[string]any
		_ = json.NewDecoder(request.Body).Decode(&outgoing)
		nativeBodies = append(nativeBodies, outgoing)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"responseId":"resp_summary","modelVersion":"gemini-3.5-flash","candidates":[{"content":{"parts":[{"text":"Keep the current worktree and next step."}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact",
		Body: []byte(`{"model":"gemini-3.8-flash","input":[{"type":"message","role":"user","content":"summarize"}],"tools":[{"type":"function","name":"ignored"}],"stream":true}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Godex-Compact-Mode") != "semantic" ||
		response.Header.Get("X-Godex-Compact-Provider") != "gemini" || !strings.Contains(string(body), "Keep the current worktree") {
		t.Fatalf("compact response = status:%d headers:%v body:%s", response.StatusCode, response.Header, body)
	}
	if len(paths) != 1 || paths[0] != "/models/chat-compression-default:generateContent" {
		t.Fatalf("compact upstream paths = %#v", paths)
	}
	if len(nativeBodies) != 1 || len(nativeBodies[0]["contents"].([]any)) == 0 {
		t.Fatalf("compact native body = %#v", nativeBodies)
	}
	system, _ := nativeBodies[0]["systemInstruction"].(map[string]any)
	parts, _ := system["parts"].([]any)
	encoded, _ := json.Marshal(parts)
	if !strings.Contains(string(encoded), "current worktree state") {
		t.Fatalf("compact instruction lost current-worktree requirement: %#v", nativeBodies[0])
	}
}

func TestGeminiCompactUsesLocalFallbackOnRequestFailure(t *testing.T) {
	transport, err := NewRuntimeTransport("https://gemini.example.test/v1beta", "fixture-key", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact", Body: []byte(`{"input":"not-an-array"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Godex-Compact-Mode") != "local-fallback" || response.Header.Get("X-Godex-Compact-Degraded") != "true" ||
		response.Header.Get("X-Godex-Compact-Reason") != "invalid-request" {
		t.Fatalf("compact fallback headers = %v", response.Header)
	}
}
