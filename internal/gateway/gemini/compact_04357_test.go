package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04357GeminiSemanticCompactPreservesActiveUserAndLatestToolResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"responseId":"resp_compact","modelVersion":"gemini-2.5-pro","candidates":[{"content":{"parts":[{"text":"Goal: keep working.\nTests: focused suite passed."}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	body := []byte(`{
		"model":"gemini-2.5-pro",
		"input":[
			{"type":"function_call_output","call_id":"call-before","output":"before-user"},
			{"type":"message","role":"user","content":"older user request"},
			{"type":"function_call_output","call_id":"call-old","output":"after-older"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"  Complete the compact fix.  "}]},
			{"type":"function_call_output","call_id":"call-first","output":"first-after-active"},
			{"type":"message","role":"assistant","content":"assistant note"},
			{"type":"custom_tool_call_output","call_id":"call-latest","output":{"text":"  focused tests passed  "}}
		]
	}`)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact", Body: body,
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	text := compactResponseText(t, value)
	for _, want := range []string{
		"Active user request that must still be completed:",
		"Complete the compact fix.",
		"Latest tool result after the active request:",
		"focused tests passed",
		"Semantic continuation summary:",
		"Goal: keep working.",
		"Tests: focused suite passed.",
		"Continue the active user request. Do not merely acknowledge repository, optimizer, or environment instructions.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("semantic compact missing %q:\n%s", want, text)
		}
	}
	for _, stale := range []string{"older user request", "after-older", "first-after-active"} {
		if strings.Contains(text, stale) {
			t.Fatalf("semantic compact retained stale %q:\n%s", stale, text)
		}
	}
}

func TestProdex04357GeminiSemanticCompactUsesLatestLocalShellToolAfterActiveUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"responseId":"resp_compact","candidates":[{"content":{"parts":[{"text":"semantic state"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	body := []byte(`{"input":[
		{"type":"message","role":"user","content":"finish this"},
		{"type":"function_call_output","call_id":"call-older","output":"older tool"},
		{"type":"local_shell_call_output","call_id":"call-shell","content":{"output":"latest shell result"}}
	]}`)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact", Body: body,
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	text := compactResponseText(t, value)
	if !strings.Contains(text, "latest shell result") || strings.Contains(text, "older tool") {
		t.Fatalf("latest-tool planning mismatch:\n%s", text)
	}
}

func TestProdex04357GeminiSemanticCompactPreservesTailOfLargeUTF8ActiveRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"responseId":"resp_compact","candidates":[{"content":{"parts":[{"text":"Keep working."}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	active := strings.Repeat("月 filler ", 500) + "FINAL_ACTION_MARKER"
	requestValue := map[string]any{
		"input": []any{map[string]any{"type": "message", "role": "user", "content": active}},
	}
	requestBody, _ := json.Marshal(requestValue)
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact", Body: requestBody,
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	text := compactResponseText(t, value)
	if !strings.Contains(text, "[... middle truncated ...]") ||
		!strings.Contains(text, "FINAL_ACTION_MARKER") ||
		!strings.Contains(text, "Keep working.") {
		t.Fatalf("large active request lost edge context:\n%s", text)
	}
	if !utf8.ValidString(text) {
		t.Fatal("semantic compact output is not valid UTF-8")
	}
}

func compactResponseText(t *testing.T, value map[string]any) string {
	t.Helper()
	output, ok := value["output"].([]any)
	if !ok || len(output) == 0 {
		t.Fatalf("compact output = %#v", value)
	}
	message, _ := output[0].(map[string]any)
	content, _ := message["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("compact message content = %#v", message)
	}
	part, _ := content[0].(map[string]any)
	text, _ := part["text"].(string)
	return text
}
