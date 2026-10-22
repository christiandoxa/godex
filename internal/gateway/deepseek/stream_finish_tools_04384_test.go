package deepseek

import (
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384DeepSeekFinishCompletesToolsBeforeDoneAndUsage(t *testing.T) {
	for _, finish := range []string{`"tool_calls"`, `""`, `"  stop  "`} {
		t.Run(finish, func(t *testing.T) {
			state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
			first, _, err := state.observe([]byte(`{"id":"chat-finish","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-finish","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":` + finish + `}]}`))
			if err != nil || strings.Count(string(first), "event: response.output_item.done") != 1 || strings.Contains(string(first), "event: response.completed") {
				t.Fatalf("tool finish did not complete its item immediately: error=%v output=%s", err, first)
			}
			usage, _, err := state.observe([]byte(`{"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
			if err != nil || strings.Contains(string(usage), "event: response.output_item.done") || strings.Contains(string(usage), "event: response.completed") {
				t.Fatalf("trailing usage duplicated/completed tools: error=%v output=%s", err, usage)
			}
			done, _, err := state.observe([]byte("[DONE]"))
			if err != nil || strings.Count(string(done), "event: response.completed") != 1 || strings.Contains(string(done), "event: response.output_item.done") ||
				!strings.Contains(string(done), `"call_id":"call-finish"`) || !strings.Contains(string(done), `"finish_reason":`+finish) || !strings.Contains(string(done), `"input_tokens":2`) {
				t.Fatalf("DONE lost final tool/finish/usage or duplicated item: error=%v output=%s", err, done)
			}
		})
	}
}

func TestProdex04384DeepSeekInvalidToolFailsAtFinishWithoutDone(t *testing.T) {
	for _, finish := range []string{`"tool_calls"`, `""`} {
		state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
		output, _, err := state.observe([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-bad","function":{"name":"lookup","arguments":"{bad"}}]},"finish_reason":` + finish + `}]}`))
		if err != nil || strings.Count(string(output), "event: response.failed") != 1 || strings.Contains(string(output), "event: response.output_item.done") {
			t.Fatalf("invalid tool was not failed at finish: error=%v output=%s", err, output)
		}
		if late, supported, err := state.observe([]byte("[DONE]")); err != nil || supported || len(late) != 0 {
			t.Fatalf("terminal failure was replayed at DONE: output=%s supported=%v error=%v", late, supported, err)
		}
	}
}

func TestProdex04384DeepSeekAbsentFinishDoesNotCompleteTool(t *testing.T) {
	for _, field := range []string{"", `,"finish_reason":null`, `,"finish_reason":false`} {
		state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
		output, _, err := state.observe([]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-later","function":{"name":"lookup","arguments":"{}"}}]}` + field + `}]}`))
		if err != nil || strings.Contains(string(output), "event: response.output_item.done") || strings.Contains(string(output), "event: response.failed") {
			t.Fatalf("missing/non-string finish prematurely completed tool: output=%s error=%v", output, err)
		}
		output, _, err = state.observe([]byte("[DONE]"))
		if err != nil || strings.Count(string(output), "event: response.output_item.done") != 1 || strings.Count(string(output), "event: response.completed") != 1 {
			t.Fatalf("DONE did not finish pending tools: output=%s error=%v", output, err)
		}
	}
}
