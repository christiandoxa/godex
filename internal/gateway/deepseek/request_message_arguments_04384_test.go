package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekMessageArgumentsPresence(t *testing.T) {
	for _, fixture := range []struct{ name, fields, want string }{
		{"absent", "", `{}`},
		{"null", `,"arguments":null`, `null`},
		{"input ignored", `,"input":{"ignored":true}`, `{}`},
		{"null priority", `,"arguments":null,"input":{"ignored":true}`, `null`},
		{"structured", `,"arguments":{"z":1e0,"a":"<>&/🌍"}`, `{"a":"<>&/🌍","z":1.0}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := `{"input":[{"type":"message","role":"assistant","content":"preface","tool_calls":[{"id":"call_synthetic","type":"function","function":{"name":"lookup"` + fixture.fields + `}}]},{"type":"function_call_output","call_id":"call_synthetic","output":"done"}]}`
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
			if len(request.Messages) != 2 || len(request.Messages[0].ToolCalls) != 1 || request.Messages[0].ToolCalls[0].Function.Arguments != fixture.want {
				t.Fatal("message call arguments lost absence default, null presence or Serde bytes")
			}
		})
	}
}
