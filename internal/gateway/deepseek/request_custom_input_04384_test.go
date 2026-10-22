package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekCustomInputStringWrapper(t *testing.T) {
	text := "<>&/🌍\u2028\u2029"
	for _, mode := range []string{"simple", "normal"} {
		for _, fixture := range []struct{ name, fields, want string }{
			{"absent", "", `{"input":""}`},
			{"null", `,"input":null`, `{"input":"null"}`},
			{"string", `,"input":"synthetic patch"`, `{"input":"synthetic patch"}`},
			{"empty", `,"input":""`, `{"input":""}`},
			{"escaped", `,"input":"line\n\"quoted\"\\slash"`, `{"input":"line\n\"quoted\"\\slash"}`},
			{"unicode", `,"input":"` + text + `"`, `{"input":"` + text + `"}`},
			{"object", `,"input":{"z":1e0,"a":"` + text + `"}`, `{"input":"{\"a\":\"` + text + `\",\"z\":1.0}"}`},
			{"array", `,"input":[null,false,1e0,-0]`, `{"input":"[null,false,1.0,-0.0]"}`},
			{"bool", `,"input":false`, `{"input":"false"}`},
			{"u64", `,"input":18446744073709551615`, `{"input":"18446744073709551615"}`},
			{"arguments ignored", `,"arguments":"ignored"`, `{"input":""}`},
			{"input priority", `,"arguments":"ignored","input":"synthetic patch"`, `{"input":"synthetic patch"}`},
			{"null priority", `,"arguments":"ignored","input":null`, `{"input":"null"}`},
			{"nested ignored", `,"function":{"arguments":{"ignored":true}}`, `{"input":""}`},
		} {
			t.Run(mode+"/"+fixture.name, func(t *testing.T) {
				prefix, callIndex := "", 0
				if mode == "normal" {
					prefix, callIndex = `{"type":"message","role":"developer","content":"synthetic instruction"},`, 1
				}
				input := `{"input":[` + prefix + `{"type":"custom_tool_call","call_id":"call_synthetic","name":"lookup"` + fixture.fields + `},{"type":"custom_tool_call_output","call_id":"call_synthetic","output":"done"}]}`
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
				if len(request.Messages) != callIndex+2 || len(request.Messages[callIndex].ToolCalls) != 1 || request.Messages[callIndex].ToolCalls[0].Function.Arguments != fixture.want {
					t.Fatal("custom tool input lost string wrapper, absence default, input priority or Serde bytes")
				}
			})
		}
	}
}
