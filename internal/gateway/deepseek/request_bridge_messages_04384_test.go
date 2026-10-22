package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekBridgeWireAndPendingMessages(t *testing.T) {
	embedded := `{"type":"message","role":"assistant","content":"preface","tool_calls":[{"id":"call_synthetic","type":"function","function":{"name":"lookup","arguments":"{}"}}]}`
	top := `{"type":"function_call","call_id":"call_synthetic","name":"lookup","arguments":"{}"}`
	developer := `{"type":"message","role":"developer","content":"synthetic instruction"},`
	system := `{"content":"synthetic instruction","role":"system"},`
	user := `{"content":"same request","role":"user"}`
	preface := `{"content":"preface","role":"assistant"}`
	call := `,"role":"assistant","tool_calls":[{"function":{"arguments":"{}","name":"lookup"},"id":"call_synthetic","type":"function"}]}`
	output := `,{"content":"done","role":"tool","tool_call_id":"call_synthetic"},`
	rawWire := `[{"content":"preface"` + call + output + user + `]`
	normal := `[` + system + preface + `,` + user + `]`
	topWire := `[` + system + `{"content":""` + call + output + user + `]`
	for _, fixture := range []struct{ name, prefix, item, wire, pending string }{
		{"simple", "", embedded, rawWire, `[` + preface + `,` + user + `]`},
		{"normal", developer, embedded, normal, normal},
		{"top-level control", developer, top, topWire, topWire},
		{"standalone control", "", embedded, rawWire, `null`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := `{"input":[` + fixture.prefix + fixture.item + `,{"type":"function_call_output","call_id":"call_synthetic","output":"done"},{"type":"message","role":"user","content":"same request"}]}`
			var contains func(string) bool
			if fixture.name != "standalone control" {
				contains = func(string) bool { return false }
			}
			translated, err := translateResponsesRequestWithHistory([]byte(body), RequestOptions{}, nil, contains)
			if err != nil {
				t.Fatal(err)
			}
			var wire struct{ Messages json.RawMessage }
			if err := json.Unmarshal(translated.Body, &wire); err != nil {
				t.Fatal(err)
			}
			if string(wire.Messages) != fixture.wire {
				t.Fatal("wire request changed embedded-call projection or top-level calls")
			}
			pending, err := json.Marshal(translated.conversationMessages)
			if err != nil {
				t.Fatal(err)
			}
			if string(pending) != fixture.pending {
				t.Fatal("normal pending history retained embedded calls or lost valid upstream history")
			}
		})
	}
}
