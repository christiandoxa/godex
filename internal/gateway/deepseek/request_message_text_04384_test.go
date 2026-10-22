package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekMessageTextViews(t *testing.T) {
	user := `{"content":"same request","role":"user"}`
	system := `{"content":"synthetic instruction","role":"system"},`
	empty := `[{"content":"","role":"user"}]`
	for _, fixture := range []struct{ name, input, wire, pending string }{
		{"simple alias", `{"type":"message","role":"user","text":"alias text"},{"role":"user","content":"same request"}`, `[{"content":"alias text","role":"user"},` + user + `]`, `[` + user + `]`},
		{"simple empty", `{"type":"message","role":"user","content":""},{"role":"user","content":"same request"}`, `[{"content":"","role":"user"},` + user + `]`, `[` + user + `]`},
		{"normal empty", `{"type":"message","role":"developer","content":"synthetic instruction"},{"type":"message","role":"user","content":""},{"role":"user","content":"same request"}`, `[` + system + user + `]`, `[` + system + user + `]`},
		{"normal alias", `{"type":"message","role":"developer","content":"synthetic instruction"},{"type":"message","role":"user","text":"alias text"},{"role":"user","content":"same request"}`, `[` + system + user + `]`, `[` + system + user + `]`},
		{"simple only empty", `{"type":"message","role":"user","content":""}`, empty, empty},
		{"normal only empty", `{"type":"message","role":"developer","content":""}`, empty, empty},
		{"simple only alias", `{"type":"message","role":"user","text":"alias text"}`, `[{"content":"alias text","role":"user"}]`, empty},
		{"visible control", `{"role":"user","content":"same request"}`, `[` + user + `]`, `[` + user + `]`},
		{"standalone empty control", `{"type":"message","role":"user","content":""}`, `[]`, `null`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			contains := func(string) bool { return false }
			if fixture.name == "standalone empty control" {
				contains = nil
			}
			translated, err := translateResponsesRequestWithHistory([]byte(`{"input":[`+fixture.input+`]}`), RequestOptions{}, nil, contains)
			if err != nil {
				t.Fatal(err)
			}
			var body struct{ Messages json.RawMessage }
			if err := json.Unmarshal(translated.Body, &body); err != nil {
				t.Fatal(err)
			}
			pending, err := json.Marshal(translated.conversationMessages)
			if err != nil {
				t.Fatal(err)
			}
			if string(body.Messages) != fixture.wire || string(pending) != fixture.pending {
				t.Fatal("message text, blank handling or normal pending fallback changed")
			}
		})
	}
}
