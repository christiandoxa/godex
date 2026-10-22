package gemini

import (
	"errors"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiChatJSONAdmissionReturnsTerminalPreparationError(t *testing.T) {
	for _, fixture := range []struct{ name, body string }{
		{"null", `null`},
		{"array", `[]`},
		{"number", `7`},
		{"boolean", `true`},
		{"string", `"opaque"`},
		{"syntax", `{"input":`},
		{"trailing", `{"input":"same request"} {}`},
		{"surrogate", `{"input":"same request","unused":"\ud800"}`},
		{"utf8", "{\"input\":\"same request\",\"unused\":\"" + string([]byte{0xff}) + "\"}"},
		{"overflow", `{"input":"same request","unused":1e309}`},
		{"depth", `{"input":"same request","unused":` + strings.Repeat("[", 140) + "0" + strings.Repeat("]", 140) + "}"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, _, err := normalizeGeminiChatInput043711([]byte(fixture.body))
			var failure *proxymodel.Error
			status, message := 502, "upstream request failed"
			if fixture.name == "null" || fixture.name == "array" || fixture.name == "number" || fixture.name == "boolean" || fixture.name == "string" {
				status, message = 500, ""
			}
			if !errors.As(err, &failure) || failure.StatusCode != status || failure.Message != message {
				t.Fatalf("invalid complete request lost terminal preparation error: %v", err)
			}
		})
	}
}
