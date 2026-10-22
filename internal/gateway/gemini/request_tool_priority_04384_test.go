package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiToolValidationPrecedesOtherShapeErrors(t *testing.T) {
	for _, fixture := range []struct{ name, input, extra string }{
		{"stream", `"same request"`, `,"stream":null`},
		{"temperature", `"same request"`, `,"temperature":null`},
		{"tokens", `"same request"`, `,"max_tokens":0`},
		{"user", `"same request"`, `,"user_id":null`},
		{"input", "7", ""},
		{"reasoning", `"same request"`, `,"reasoning":"opaque"`},
		{"serde", `"same request"`, `,"unused":"\ud800"`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := `{"model":"gemini-3.1-pro-preview","input":` + fixture.input + `,"tools":[{"type":"function","function":{"name":"synthetic_lookup","parameters":false}}]` + fixture.extra + `}`
			_, _, err := normalizeGeminiChatInput043711([]byte(body))
			wantStatus, wantMessage := 400, "invalid_tool_declaration: Gemini request field `tools[0].function.parameters` must be an object"
			if fixture.name == "serde" {
				wantStatus, wantMessage = 502, "upstream request failed"
			}
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != wantStatus || failure.Message != wantMessage {
				t.Fatalf("combined-error ordering differs: error=%v want status=%d message=%s", err, wantStatus, wantMessage)
			}
		})
	}
}
