package chatcompat

import (
	"io"
	"strings"
	"testing"
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

func TestChatSSETranslatesChunkedEvents(t *testing.T) {
	input := io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"))
	output, err := io.ReadAll(ChatSSE(input))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "response.output_text.delta") || !strings.Contains(string(output), "response.completed") {
		t.Fatalf("translated SSE = %q", output)
	}
}
