package gemini

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384GeminiChatStreamToolSnapshotsFollowFinishAndDone(t *testing.T) {
	for _, fixture := range []struct {
		name, arguments string
		finish, done    bool
		cached          bool
	}{
		{"finish", `{}`, true, true, true},
		{"finish EOF", `{}`, true, false, true},
		{"DONE only", `{}`, false, true, true},
		{"unfinished", `{}`, false, false, false},
		{"invalid", `{bad`, true, true, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := chatcompat.NewConversationStore()
			owned := store.Scoped("synthetic owner")
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1, conversation: &geminiChatConversation04384{store: owned, messages: []any{map[string]any{"role": "user", "content": "original"}}}}
			var wire bytes.Buffer
			packets := []string{`{"choices":[{"delta":{"tool_calls":[{"index":2,"id":"call_synthetic","function":{"name":"lookup","arguments":"` + fixture.arguments + `"}}]}}]}`}
			if fixture.finish {
				packets = append(packets, `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
			}
			if fixture.done {
				packets = append(packets, "[DONE]")
			}
			for _, packet := range packets {
				if err := state.accept(&wire, []byte(packet)); err != nil {
					t.Fatal(err)
				}
			}
			if owned.Contains(state.id) != fixture.cached || store.Contains(state.id) || store.Scoped("foreign owner").Contains(state.id) {
				t.Fatal("tool snapshot lost its finish/DONE/validation or credential boundary")
			}
			if !fixture.cached {
				return
			}
			history := owned.History(state.id)
			if len(history) != 2 || history[0].(map[string]any)["content"] != "original" {
				t.Fatal("tool snapshot lost original messages")
			}
			assistant := history[1].(map[string]any)
			calls, _ := assistant["tool_calls"].([]any)
			if assistant["content"] != "" || len(calls) != 1 || calls[0].(map[string]any)["id"] != "call_synthetic" || calls[0].(map[string]any)["function"].(map[string]any)["arguments"] != `{}` {
				t.Fatal("tool snapshot lost exact assistant/call projection")
			}
			assistant["content"] = "mutated"
			if owned.History(state.id)[1].(map[string]any)["content"] != "" {
				t.Fatal("tool snapshot leaked a mutable history alias")
			}
		})
	}
}

func TestProdex04384GeminiChatToolSnapshotSurvivesFinishThenEOF(t *testing.T) {
	store := chatcompat.NewConversationStore()
	conversation := &geminiChatConversation04384{store: store, messages: []any{map[string]any{"role": "user", "content": "original"}}}
	wire := "data: " + `{"choices":[{"delta":{"tool_calls":[{"id":"call_synthetic","function":{"name":"lookup","arguments":"{}"}}]}}]}` + "\n\ndata: " + `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"
	body := translateGeminiChatCompatibleStream043711(context.Background(), &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil, conversation).Body
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil || !strings.Contains(string(data), "event: response.failed") || len(store.FindHistoryByCallID("call_synthetic")) != 2 {
		t.Fatal("later EOF erased an already validated finish snapshot")
	}
}

func TestProdex04384GeminiToolSnapshotRetainsLastValidatedBoundary(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		store := chatcompat.NewConversationStore()
		state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1, conversation: &geminiChatConversation04384{store: store}}
		var wire bytes.Buffer
		for _, packet := range []string{
			`{"choices":[{"delta":{"tool_calls":[{"id":"call_synthetic","function":{"name":"lookup","arguments":"{}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		} {
			if err := state.accept(&wire, []byte(packet)); err != nil {
				t.Fatal(err)
			}
		}
		packet := `{"choices":[{"delta":{"content":"after finish"}}]}`
		if invalid {
			packet = `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"bad"}}]}}]}`
		}
		for _, packet := range []string{packet, "[DONE]"} {
			if err := state.accept(&wire, []byte(packet)); err != nil {
				t.Fatal(err)
			}
		}
		history := store.History(state.id)
		if len(history) != 1 {
			t.Fatal("later packet discarded a validated snapshot")
		}
		assistant := history[0].(map[string]any)
		wantContent := "after finish"
		if invalid {
			wantContent = ""
		}
		call := assistant["tool_calls"].([]any)[0].(map[string]any)
		if assistant["content"] != wantContent || call["function"].(map[string]any)["arguments"] != `{}` {
			t.Fatal("DONE failed to update valid content or replaced the prior snapshot with invalid arguments")
		}
	}
}
