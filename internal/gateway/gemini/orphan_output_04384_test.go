package gemini

import (
	"reflect"
	"testing"
)

func TestProdex04384GeminiAdjacencyDropsAllUnmatchedToolOutputsWhenIndexed(t *testing.T) {
	for _, fixture := range []struct{ name, messages, want string }{
		{"valid-orphan", `[{"role":"tool","tool_call_id":"orphan","content":"drop"}]`, `[{"role":"user","content":""}]`},
		{"no-valid-output-id", `[{"role":"tool","tool_call_id":" ","content":"keep"}]`, `[{"role":"tool","tool_call_id":" ","content":"keep"}]`},
		{"valid-and-blank-orphans", `[{"role":"tool","tool_call_id":"orphan","content":"drop"},{"role":"tool","tool_call_id":" ","content":"also drop"}]`, `[{"role":"user","content":""}]`},
		{"matched-and-orphan", `[{"role":"tool","tool_call_id":"orphan","content":"drop"},{"role":"assistant","tool_calls":[{"id":"matched"}]},{"role":"tool","tool_call_id":"matched","content":"keep"}]`, `[{"role":"assistant","content":"","tool_calls":[{"id":"matched"}]},{"role":"tool","tool_call_id":"matched","content":"keep"}]`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			got, err := filterGeminiOrphanToolMessages043711([]byte(`{"messages":` + fixture.messages + `}`))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := decodeGeminiChatRequest04384(got)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := decodeGeminiChatRequest04384([]byte(`{"messages":` + fixture.want + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("orphan output handling differs: got=%s want=%s", got, fixture.want)
			}
		})
	}
}
