package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekCallArgumentPresenceAndSerdeWire(t *testing.T) {
	text := "<>&/🌍\u2028\u2029"
	for _, fixture := range []struct{ name, kind, fields, want string }{
		{"absent", "function_call", "", `{}`},
		{"null", "function_call", `,"arguments":null`, `null`},
		{"input", "function_call", `,"input":{"v":1}`, `{"v":1}`},
		{"nested", "function_call", `,"function":{"name":"lookup","arguments":{"v":1}}`, `{"v":1}`},
		{"nested null", "function_call", `,"function":{"name":"lookup","arguments":null}`, `null`},
		{"null priority", "function_call", `,"arguments":null,"input":{"v":1}`, `null`},
		{"object", "function_call", `,"arguments":{"text":"` + text + `"}`, `{"text":"` + text + `"}`},
		{"array", "function_call", `,"arguments":[null,false,1]`, `[null,false,1]`},
		{"u64", "function_call", `,"arguments":{"n":18446744073709551615}`, `{"n":18446744073709551615}`},
		{"float", "function_call", `,"arguments":{"n":1e0}`, `{"n":1.0}`},
		{"negative zero", "function_call", `,"arguments":-0`, `-0.0`},
		{"opaque string", "function_call", `,"arguments":" { \"x\": 1 } "`, ` { "x": 1 } `},
		{"opaque escape", "function_call", `,"arguments":"{\"escaped\":\"\\\\u003c\"}"`, `{"escaped":"\\u003c"}`},
		{"MCP absent", "mcp_call", "", `{}`},
		{"MCP null priority", "mcp_call", `,"arguments":null,"input":{"v":1}`, `null`},
		{"MCP nested", "mcp_call", `,"function":{"name":"lookup","arguments":{"v":1}}`, `{"v":1}`},
		{"custom control", "custom_tool_call", `,"input":"synthetic patch"`, `{"input":"synthetic patch"}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := `{"input":[{"type":"message","role":"developer","content":"synthetic instruction"},{"type":"` + fixture.kind + `","call_id":"call_synthetic","name":"lookup"` + fixture.fields + `},{"type":"function_call_output","call_id":"call_synthetic","output":"done"}]}`
			body, err := ResponsesRequest([]byte(input), RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Messages []struct {
					ToolCalls []struct{ Function struct{ Arguments string } } `json:"tool_calls"`
				}
			}
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			if len(request.Messages) != 3 || len(request.Messages[1].ToolCalls) != 1 || request.Messages[1].ToolCalls[0].Function.Arguments != fixture.want {
				t.Fatal("call argument presence, alias priority or Serde bytes changed")
			}
		})
	}
}
