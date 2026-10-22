package deepseek

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384StreamToolThoughtSignaturePreservesNonblankBytes(t *testing.T) {
	for _, signature := range []string{"sig", " sig ", "\t sig_🧭 \n", "\u3000sig\u3000", "", " \t\n\u3000"} {
		call := map[string]any{"extra_content": map[string]any{"google": map[string]any{"thought_signature": signature}}}
		want := signature
		if strings.TrimSpace(signature) == "" {
			want = ""
		}
		if got := deepSeekStreamThoughtSignature(call); got != want {
			t.Errorf("signature changed: got=%q want=%q", got, want)
		}
	}
}

func TestProdex04384StreamToolThoughtSignatureSurvivesItemsAndSnapshot(t *testing.T) {
	const signature = "\t sig_🧭 \n"
	state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
	chunk, err := json.Marshal(map[string]any{
		"id": "chat-signature", "created": 1, "choices": []any{map[string]any{
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call-signature", "function": map[string]any{"name": "lookup", "arguments": `{}`},
				"extra_content": map[string]any{"google": map[string]any{"thought_signature": signature}},
			}}}, "finish_reason": "tool_calls",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, _, err := state.observe(chunk)
	if err != nil {
		t.Fatal(err)
	}
	terminal, _, err := state.observe([]byte("[DONE]"))
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(signature)
	for _, output := range [][]byte{events, terminal} {
		if !strings.Contains(string(output), `"gemini_thought_signature":`+string(quoted)) {
			t.Errorf("tool completion changed thought signature: %s", output)
		}
	}
	message, err := json.Marshal(state.assistantMessage())
	if err != nil || !strings.Contains(string(message), `"thought_signature":`+string(quoted)) {
		t.Errorf("conversation snapshot changed thought signature: %s error=%v", message, err)
	}
	stream := deepSeekChatSSE(io.NopCloser(strings.NewReader("data: " + string(chunk) + "\n\ndata: [DONE]\n\n")))
	defer stream.Close()
	output, err := io.ReadAll(stream)
	if err != nil || strings.Count(string(output), `"gemini_thought_signature":`+string(quoted)) != 2 {
		t.Fatalf("stream lost signature in done/final output: %s error=%v", output, err)
	}
}

func TestProdex04384StreamToolBlankSignatureDoesNotReplaceEarlierValue(t *testing.T) {
	state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
	for _, signature := range []string{"original", " \u3000\t", ""} {
		state.observeToolCallDelta(map[string]any{
			"index": json.Number("0"), "function": map[string]any{"name": "lookup"},
			"extra_content": map[string]any{"google": map[string]any{"thought_signature": signature}},
		})
	}
	if got := state.toolCalls[0].thoughtSignature; got != "original" {
		t.Fatalf("blank signature replaced earlier valid value: %q", got)
	}
}
