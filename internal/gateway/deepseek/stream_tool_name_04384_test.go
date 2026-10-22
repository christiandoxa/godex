package deepseek

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384StreamToolNamespaceSplittingRetainsOriginalBytes(t *testing.T) {
	for _, fixture := range []struct{ source, namespace, name string }{
		{"lookup", "", "lookup"},
		{" lookup ", "", " lookup "},
		{" tool_search ", "", " tool_search "},
		{" ns -- tool ", " ns ", " tool "},
		{"ns-- ", "", "ns-- "},
		{" --lookup", "", " --lookup"},
		{"mcp__scope__ tool ", "mcp__scope", " tool "},
		{"mcp____read", "mcp__", "read"},
		{" mcp__scope__tool ", "", " mcp__scope__tool "},
		{"mcp__scope__ ", "", "mcp__scope__ "},
	} {
		if namespace, name := deepSeekStreamSplitToolName(fixture.source); namespace != fixture.namespace || name != fixture.name {
			t.Errorf("split %q = %q/%q want=%q/%q", fixture.source, namespace, name, fixture.namespace, fixture.name)
		}
	}
}

func TestProdex04384StreamToolPaddedSpecialNameRemainsFunctionCall(t *testing.T) {
	chunk := `{"id":"chat-name","created":1,"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-name","function":{"name":" tool_search ","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
	body := deepSeekChatSSE(io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")))
	defer body.Close()
	output, err := io.ReadAll(body)
	if err != nil || strings.Count(string(output), `"name":" tool_search "`) != 3 ||
		strings.Count(string(output), `"type":"function_call"`) != 3 ||
		strings.Count(string(output), "event: ") != 5 || strings.Contains(string(output), `"type":"tool_search_call"`) {
		t.Fatalf("padded special tool was reclassified or trimmed: output=%s error=%v", output, err)
	}
}

func TestProdex04384StreamToolNamePresenceReplacesEarlierValue(t *testing.T) {
	for _, name := range []any{"", " \u3000\t", nil, 7} {
		state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
		first := []byte(`{"id":"chat-name","created":1,"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-name","function":{"name":"lookup","arguments":"{}"}}]}}]}`)
		if _, _, err := state.observe(first); err != nil {
			t.Fatal(err)
		}
		last, err := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"delta":         map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"name": name}}}},
			"finish_reason": "tool_calls",
		}}})
		if err != nil {
			t.Fatal(err)
		}
		output, _, err := state.observe(last)
		if err != nil {
			t.Fatal(err)
		}
		if _, present := name.(string); present {
			if !strings.Contains(string(output), "DeepSeek streamed a tool call without a function name at index 0") ||
				strings.Contains(string(output), "event: response.output_item.done") {
				t.Errorf("present blank name failed to invalidate prior name: name=%q output=%s", name, output)
			}
		} else if state.toolCalls[0].name != "lookup" || !strings.Contains(string(output), "event: response.output_item.done") {
			t.Errorf("non-string name replaced prior name: name=%v output=%s", name, output)
		}
	}
}

func TestProdex04384StreamToolInvalidArgumentsNameIsTrimmedOnlyInDiagnostic(t *testing.T) {
	call := &deepSeekChatStreamToolCall{name: " lookup "}
	call.arguments.WriteString("{")
	err := deepSeekValidateStreamToolCall(0, call)
	if err == nil || err.Error() != "DeepSeek streamed malformed JSON arguments for tool call `lookup` at index 0: invalid JSON" {
		t.Fatalf("padded tool diagnostic differs: %v", err)
	}
}
