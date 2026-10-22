package deepseek

import "testing"

func TestProdex04384DeepSeekLogprobsAdmission(t *testing.T) {
	for _, fixture := range []struct{ name, fields, want string }{
		{"false", `"logprobs":false`, ""},
		{"zero", `"logprobs":true,"top_logprobs":0`, ""},
		{"maximum", `"logprobs":true,"top_logprobs":20`, ""},
		{"null_flag", `"logprobs":null`, "DeepSeek logprobs must be a boolean"},
		{"string_flag", `"logprobs":"true"`, "DeepSeek logprobs must be a boolean"},
		{"null_top", `"logprobs":true,"top_logprobs":null`, "DeepSeek top_logprobs must be an integer"},
		{"negative", `"logprobs":true,"top_logprobs":-1`, "DeepSeek top_logprobs must be an integer"},
		{"negative_zero", `"logprobs":true,"top_logprobs":-0`, "DeepSeek top_logprobs must be an integer"},
		{"fraction", `"logprobs":true,"top_logprobs":1.0`, "DeepSeek top_logprobs must be an integer"},
		{"exponent", `"logprobs":true,"top_logprobs":1e0`, "DeepSeek top_logprobs must be an integer"},
		{"over_maximum", `"logprobs":true,"top_logprobs":21`, "DeepSeek top_logprobs must be <= 20"},
		{"unsigned", `"logprobs":true,"top_logprobs":18446744073709551615`, "DeepSeek top_logprobs must be <= 20"},
		{"missing_flag", `"top_logprobs":2`, "DeepSeek top_logprobs requires logprobs=true"},
		{"false_flag", `"logprobs":false,"top_logprobs":0`, "DeepSeek top_logprobs requires logprobs=true"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body, err := ResponsesRequest([]byte(`{"model":"deepseek-v4-pro","input":"same request",`+fixture.fields+`}`), RequestOptions{})
			if fixture.want == "" {
				if err != nil || len(body) == 0 {
					t.Fatalf("valid logprobs rejected: %v", err)
				}
				return
			}
			if err == nil || err.Error() != fixture.want {
				t.Fatalf("logprobs admission = %v, want %q", err, fixture.want)
			}
		})
	}
}
