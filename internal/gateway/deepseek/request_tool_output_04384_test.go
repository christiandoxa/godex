package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekToolOutputContent(t *testing.T) {
	for _, fixture := range []struct{ name, kind, value, want string }{
		{"string", "function_call_output", `"synthetic result"`, `"synthetic result"`},
		{"null", "function_call_output", `null`, `"null"`},
		{"object", "function_call_output", `{"z":1e0,"a":"<>&/🌍"}`, `"{\"a\":\"<>&/🌍\",\"z\":1.0}"`},
		{"empty array", "function_call_output", `[]`, `""`},
		{"strings", "function_call_output", `["first","second"]`, `"first\nsecond"`},
		{"parts", "function_call_output", `[{"text":"first"},{"input_text":"second"},{"output_text":"third"}]`, `"first\nsecond\nthird"`},
		{"empty parts", "function_call_output", `["",{"text":""},{"text":"last"}]`, `"\n\nlast"`},
		{"aliases", "function_call_output", `[{"text":null,"input_text":"first"},{"text":"","input_text":"ignored"},{"text":false,"input_text":7,"output_text":"third"}]`, `"first\n\nthird"`},
		{"skipped", "function_call_output", `[null,false,7,[],{},"last"]`, `"last"`},
		{"custom parts", "custom_tool_call_output", `[{"text":"first"},{"text":"second"}]`, `"first\nsecond"`},
		{"MCP parts", "mcp_call_output", `[{"text":"first"},{"text":"second"}]`, `"first\nsecond"`},
		{"MCP inline array", "mcp_call", `[{"text":"first"}]`, `[{"text":"first"}]`},
		{"MCP inline object", "mcp_call", `{"z":1e0,"a":"<>&/🌍"}`, `{"a":"<>&/🌍","z":1.0}`},
		{"MCP inline null", "mcp_call", `null`, `null`},
		{"MCP inline string", "mcp_call", `"synthetic result"`, `"synthetic result"`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			call := `{"type":"function_call","call_id":"call_synthetic","name":"lookup","arguments":"{}"},`
			if fixture.kind == "mcp_call" {
				call = ""
			}
			input := `{"input":[` + call + `{"type":"` + fixture.kind + `","call_id":"call_synthetic","name":"lookup","arguments":"{}","output":` + fixture.value + `}]}`
			body, err := ResponsesRequest([]byte(input), RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Messages []struct{ Content json.RawMessage }
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			if len(request.Messages) != 2 || string(request.Messages[1].Content) != fixture.want {
				t.Fatal("tool output lost text extraction, empty parts, aliases or raw MCP value")
			}
		})
	}
}
