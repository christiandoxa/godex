package chatcompat

import "testing"

func TestBridgeOutputRetentionKeepsDefaultRequestContract(t *testing.T) {
	for _, fixture := range []struct{ name, content, legacy string }{
		{"string", `"synthetic-result"`, `"synthetic-result"`},
		{"object", `{"value":1}`, `"{\"value\":1}"`},
		{"array", `[{"text":"part"}]`, `"part"`},
		{"null", "null", `"null"`},
		{"false", "false", `"false"`},
		{"large-integer", "9007199254740993", `"9007199254740993"`},
	} {
		for _, mode := range []struct {
			name string
			run  func([]byte, string, string) ([]byte, error)
		}{
			{"strict", ResponsesRequest},
			{"preserving", ResponsesRequestPreservingMessages},
		} {
			t.Run(fixture.name+"/"+mode.name, func(t *testing.T) {
				body := `{"input":[{"type":"mcp_tool_result","call_id":"synthetic-call","output":` + fixture.content + `}]}`
				got, err := mode.run([]byte(body), "synthetic-model", "")
				content := fixture.content
				if mode.name == "strict" {
					content = fixture.legacy
				}
				want := `{"messages":[{"content":` + content + `,"role":"tool","tool_call_id":"synthetic-call"}],"model":"synthetic-model","stream":false}`
				if err != nil || string(got) != want {
					t.Fatalf("bridge output retention differs: got=%s error=%v want=%s", got, err, want)
				}
			})
		}
	}
}
