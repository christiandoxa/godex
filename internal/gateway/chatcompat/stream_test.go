package chatcompat

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTranslateChatSSEDataMatchesProdexFixtures(t *testing.T) {
	tool, supported, err := TranslateChatSSEData([]byte(`{"choices":[{"delta":{"content":"ignored","tool_calls":[{"id":"call_test","function":{"name":"functions.exec_command","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`))
	if err != nil || !supported || !strings.Contains(string(tool), "response.function_call_arguments.delta") || !strings.Contains(string(tool), `rtk ls`) {
		t.Fatalf("tool event = %q, supported=%t, err=%v", tool, supported, err)
	}
	text, supported, err := TranslateChatSSEData([]byte(`{"choices":[{"delta":{"content":"東京\\nquoted"}}]}`))
	if err != nil || !supported || !strings.Contains(string(text), "response.output_text.delta") || !strings.Contains(string(text), `東京\\nquoted`) {
		t.Fatalf("text event = %q, supported=%t, err=%v", text, supported, err)
	}
	finish, supported, err := TranslateChatSSEData([]byte(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
	if err != nil || !supported || !strings.Contains(string(finish), "response.completed") {
		t.Fatalf("finish event = %q, supported=%t, err=%v", finish, supported, err)
	}
	done, supported, err := TranslateChatSSEData([]byte(`[DONE]`))
	if err != nil || !supported || !strings.Contains(string(done), "response.completed") {
		t.Fatalf("done event = %q, supported=%t, err=%v", done, supported, err)
	}
}

func TestChatSSEReasoningMetadataIsBounded(t *testing.T) {
	metadata := make(map[string]any)
	for _, part := range []string{strings.Repeat("x", streamMetadataMaxBytes-2), strings.Repeat("y", 10)} {
		body, err := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"delta": map[string]any{"reasoning_content": part},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = translateChatSSEData(body, "gemini", nil, metadata)
		if err != nil {
			t.Fatalf("translate reasoning event err=%v", err)
		}
	}
	finished, _, err := translateChatSSEData([]byte(`[DONE]`), "gemini", nil, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(finished), `"reasoning_content"`) {
		t.Fatalf("completed event omitted bounded reasoning metadata: %s", finished)
	}
	lines := strings.SplitN(string(finished), "\n", 2)
	if len(lines) != 2 {
		t.Fatalf("completed event framing = %q", finished)
	}
	data := strings.TrimSpace(strings.TrimPrefix(lines[1], "data:"))
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatal(err)
	}
	reasoning := payload["metadata"].(map[string]any)["gemini"].(map[string]any)["reasoning_content"].(string)
	if len(reasoning) != streamMetadataMaxBytes || !utf8.ValidString(reasoning) {
		t.Fatalf("reasoning metadata length=%d, want %d", len(reasoning), streamMetadataMaxBytes)
	}
}

func TestChatSSETranslatesChunkedEvents(t *testing.T) {
	input := io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	output, err := io.ReadAll(ChatSSE(input))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "response.output_text.delta") || !strings.Contains(string(output), "response.completed") {
		t.Fatalf("translated SSE = %q", output)
	}
	if got := strings.Count(string(output), "event: response.completed"); got != 1 {
		t.Fatalf("completion events = %d, want 1: %q", got, output)
	}
}

func TestChatSSECompletedMetadataPreservesRequestMetadata(t *testing.T) {
	input := io.NopCloser(strings.NewReader("data: [DONE]\n\n"))
	output, err := io.ReadAll(ChatSSEWithMetadata(input, "gemini", map[string]any{
		"client_metadata": map[string]any{"client": "codex"},
		"gemini":          map[string]any{"prompt_cache_key": "key"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\"client_metadata\":{\"client\":\"codex\"}", "\"gemini\":{\"prompt_cache_key\":\"key\"}"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("translated SSE metadata missing %q: %s", want, output)
		}
	}
}
