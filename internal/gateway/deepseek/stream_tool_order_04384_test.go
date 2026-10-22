package deepseek

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384StreamToolValidationUsesLowestInvalidIndex(t *testing.T) {
	for _, terminal := range []string{"tool_calls", "", "done"} {
		for _, missingName := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/missing-name=%v", terminal, missingName), func(t *testing.T) {
				for range 64 {
					calls := []any{}
					for _, index := range []uint64{^uint64(0), 17, 3, 0} {
						name, arguments := "shell", "{"
						if index == 0 {
							arguments = `{}`
						} else if missingName {
							name = ""
							arguments = `{}`
						}
						calls = append(calls, map[string]any{
							"index": index, "id": fmt.Sprintf("call-%d", index),
							"function": map[string]any{"name": name, "arguments": arguments},
						})
					}
					choice := map[string]any{"delta": map[string]any{"tool_calls": calls}}
					if terminal != "done" {
						choice["finish_reason"] = terminal
					}
					chunk, err := json.Marshal(map[string]any{"id": "chatcmpl-order", "created": 1, "choices": []any{choice}})
					if err != nil {
						t.Fatal(err)
					}
					body := deepSeekChatSSE(io.NopCloser(strings.NewReader("data: " + string(chunk) + "\n\ndata: [DONE]\n\n")))
					output, err := io.ReadAll(body)
					body.Close()
					if err != nil {
						t.Fatal(err)
					}
					message := "DeepSeek streamed malformed JSON arguments for tool call `shell` at index 3: invalid JSON"
					if missingName {
						message = "DeepSeek streamed a tool call without a function name at index 3"
					}
					if !strings.Contains(string(output), `"message":"`+message+`"`) ||
						strings.Count(string(output), "event: response.failed\r\n") != 1 ||
						strings.Contains(string(output), "event: response.completed") ||
						strings.Contains(string(output), "event: response.output_item.done") {
						t.Fatalf("tool validation did not select the lowest invalid index: %s", output)
					}
				}
			})
		}
	}
}

func TestProdex04384StreamToolValidationKeepsValidAndSingleFailureControls(t *testing.T) {
	state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
	for _, index := range []uint64{^uint64(0), 17, 0} {
		call := &deepSeekChatStreamToolCall{name: "shell"}
		call.arguments.WriteString(`{}`)
		state.toolCalls[index] = call
	}
	if err := state.validateToolCalls(); err != nil {
		t.Fatal(err)
	}
	state.toolCalls[17].arguments.WriteString("invalid")
	if err := state.validateToolCalls(); err == nil || !strings.Contains(err.Error(), "at index 17:") {
		t.Fatalf("single invalid tool was hidden behind a valid lower index: %v", err)
	}
}
