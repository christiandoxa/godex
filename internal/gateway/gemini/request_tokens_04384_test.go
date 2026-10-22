package gemini

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiTokenLimitPrecedenceAndAdmission(t *testing.T) {
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
			body, err := normalizeGeminiChatInput043711([]byte(`{"input":"same request",` + fixture.fields + `}`))
			if fixture.want == "" {
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("invalid token limit lost terminal admission: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body, err = chatcompat.ResponsesRequest(body, "gemini-3.1-pro-preview", "")
			if err != nil {
				t.Fatal(err)
			}
			request, err := decodeGeminiChatRequest04384(body)
			if err != nil || request["max_tokens"] != json.Number(fixture.want) {
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
