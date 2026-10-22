package gemini

import (
	"bytes"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384GeminiToolSnapshotAfterPriorSemanticFailure(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		repair  bool
		finish  bool
		invalid bool
	}{
		{name: "repaired name then finish", repair: true, finish: true},
		{name: "embedded error then valid finish", finish: true},
		{name: "embedded error then DONE"},
		{name: "still invalid finish", repair: true, finish: true, invalid: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := chatcompat.NewConversationStore()
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1, conversation: &geminiChatConversation04384{store: store}}
			first := `{"error":"synthetic error"}`
			if fixture.repair {
				first = `{"choices":[{"delta":{"content":"before","tool_calls":[{"index":2,"id":"call_synthetic","function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
			}
			packets := []string{first}
			if !fixture.invalid {
				packets = append(packets, `{"choices":[{"delta":{"content":"after","tool_calls":[{"index":2,"id":"call_synthetic","function":{"name":"lookup"}}]}}]}`)
			}
			if fixture.finish {
				packets = append(packets, `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
			}
			packets = append(packets, "[DONE]")
			var wire bytes.Buffer
			for _, packet := range packets {
				if err := state.accept(&wire, []byte(packet)); err != nil {
					t.Fatal(err)
				}
			}
			cached := fixture.finish && !fixture.invalid
			if store.Contains(state.id) != cached || strings.Count(wire.String(), "event: response.failed") != 1 || strings.Contains(wire.String(), "event: response.completed") {
				t.Fatal("snapshot eligibility borrowed the terminal-response flag or fabricated another outcome")
			}
			if cached {
				history := store.History(state.id)
				assistant := history[0].(map[string]any)
				text := "after"
				if fixture.repair {
					text = "beforeafter"
				}
				if assistant["content"] != text {
					t.Fatal("valid finish snapshot lost text received after semantic failure")
				}
				call := assistant["tool_calls"].([]any)[0].(map[string]any)
				if call["id"] != "call_synthetic" || call["function"].(map[string]any)["name"] != "lookup" {
					t.Fatal("valid post-failure snapshot lost repaired tool identity/name")
				}
			}
		})
	}
}

func TestProdex04384GeminiBoundsTextRetainedAfterSemanticFailure(t *testing.T) {
	state := geminiChatCompatibleStream043711{done: true, itemOpened: true}
	state.text.WriteString(strings.Repeat("x", geminiStreamMetadataMaxBytes04384-1))
	var wire bytes.Buffer
	if err := state.emitText(&wire, "a"); err != nil || state.text.Len() != geminiStreamMetadataMaxBytes04384 || !strings.Contains(wire.String(), `"delta":"a"`) {
		t.Fatal("post-failure text did not reach its exact retention budget")
	}
	wire.Reset()
	if err := state.emitText(&wire, "overflow"); err != nil || !state.eof || state.text.Len() != geminiStreamMetadataMaxBytes04384 || wire.Len() != 0 {
		t.Fatal("post-failure text overflow retained data or fabricated a second response failure")
	}
}
