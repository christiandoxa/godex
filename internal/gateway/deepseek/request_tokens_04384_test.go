package deepseek

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekTokenLimitPrecedenceAndAdmission(t *testing.T) {
	for _, fixture := range []struct{ name, fields, want string }{
		{"output", `"max_output_tokens":11`, "11"},
		{"maximum", `"max_tokens":23`, "23"},
		{"completion", `"max_completion_tokens":31`, "31"},
		{"output_and_maximum", `"max_output_tokens":11,"max_tokens":23`, "23"},
		{"all", `"max_output_tokens":11,"max_tokens":23,"max_completion_tokens":31`, "31"},
		{"unsigned", `"max_tokens":18446744073709551615`, "18446744073709551615"},
		{"invalid_shadow_null", `"max_output_tokens":null,"max_completion_tokens":31`, ""},
		{"invalid_shadow_zero", `"max_tokens":0,"max_completion_tokens":31`, ""},
		{"invalid_shadow_fraction", `"max_output_tokens":1.0,"max_completion_tokens":31`, ""},
		{"zero", `"max_tokens":0`, ""},
		{"negative", `"max_tokens":-1`, ""},
		{"exponent", `"max_tokens":1e3`, ""},
		{"overflow", `"max_tokens":18446744073709551616`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body, err := ResponsesRequest([]byte(`{"input":"same request",`+fixture.fields+`}`), RequestOptions{})
			if fixture.want == "" {
				if err == nil {
					t.Fatal("invalid token limit was masked by another alias")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.UseNumber()
			var request map[string]any
			if err := decoder.Decode(&request); err != nil || request["max_tokens"] != json.Number(fixture.want) {
				t.Fatalf("canonical token limit differs: request=%s error=%v", body, err)
			}
			for _, alias := range []string{"max_output_tokens", "max_completion_tokens"} {
				if _, present := request[alias]; present {
					t.Fatal("noncanonical token alias reached the provider")
				}
			}
		})
	}
}

func TestProdex04384DeepSeekParallelTrueIsIgnored(t *testing.T) {
	body, err := ResponsesRequest([]byte(`{"input":"same request","parallel_tool_calls":true}`), RequestOptions{})
	if err != nil || bytes.Contains(body, []byte("parallel_tool_calls")) {
		t.Fatalf("tolerated parallel control reached the provider: request=%s error=%v", body, err)
	}
}
