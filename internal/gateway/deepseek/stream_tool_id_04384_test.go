package deepseek

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384StreamToolIDPresenceAndFallbackReachEveryEvent(t *testing.T) {
	for _, fixture := range []struct {
		name, member, want string
		fallback           bool
	}{
		{"missing", "", "", true},
		{"null", `"id":null,`, "", true},
		{"number", `"id":7,`, "", true},
		{"empty", `"id":"",`, "", false},
		{"spaces", `"id":"  ",`, "  ", false},
		{"padded", `"id":" call-preserved ",`, " call-preserved ", false},
		{"normal", `"id":"call-control",`, "call-control", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			chunk := `{"id":"chat-tool-id","created":1,"choices":[{"delta":{"tool_calls":[{` + fixture.member + `"index":0,"function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`
			body := deepSeekChatSSEWithConversation(io.NopCloser(strings.NewReader("data: "+chunk+"\n\ndata: [DONE]\n\n")), 42, nil, nil, chatcompat.ConversationStore{})
			defer body.Close()
			output, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			want := fixture.want
			if fixture.fallback {
				decoder := sse.NewDecoder(1 << 20)
				frames := append(decoder.Feed(output), decoder.Finish()...)
				var added struct {
					Item struct {
						CallID string `json:"call_id"`
					}
				}
				if len(frames) != 5 || json.Unmarshal(frames[1], &added) != nil {
					t.Fatal("fallback stream lost its tool-added item")
				}
				want = assertDeepSeekUUIDv7(t, added.Item.CallID, "call_deepseek_")
			}
			quoted, _ := json.Marshal(want)
			if strings.Count(string(output), `"call_id":`+string(quoted)) != 4 ||
				strings.Count(string(output), "event: ") != 5 || strings.Contains(string(output), "event: response.failed") {
				t.Fatalf("tool ID presence/fallback changed: want=%q output=%s", fixture.want, output)
			}
		})
	}
}

func TestProdex04384StreamToolFallbackIsUUIDv7EvenAtMaximumIndices(t *testing.T) {
	state := newDeepSeekChatStreamState(^uint64(0), nil, nil, chatcompat.ConversationStore{})
	state.observeToolCallDelta(map[string]any{
		"index": json.Number("18446744073709551615"), "function": map[string]any{"name": "lookup"},
	})
	assertDeepSeekUUIDv7(t, state.toolCalls[^uint64(0)].id, "call_deepseek_")
}

func TestProdex04384StreamToolPresentEmptyIDReplacesEarlierIdentity(t *testing.T) {
	state := newDeepSeekChatStreamState(7, nil, nil, chatcompat.ConversationStore{})
	for _, callID := range []any{"original", nil, 7, ""} {
		state.observeToolCallDelta(map[string]any{
			"index": json.Number("2"), "id": callID, "function": map[string]any{"name": "lookup"},
		})
		if callID == nil || callID == 7 {
			if got := state.toolCalls[2].id; got != "original" {
				t.Fatalf("non-string ID replaced earlier identity: %q", got)
			}
		}
	}
	if got := state.toolCalls[2].id; got != "" {
		t.Fatalf("present empty ID did not replace earlier identity: %q", got)
	}
}
