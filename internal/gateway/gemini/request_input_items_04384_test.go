package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiInputItemsCannotBeDiscardedOrLosePrefix(t *testing.T) {
	const message = `{"role":"user","content":"same request"}`
	for _, fixture := range []struct{ name, items string }{
		{"null", `null,` + message},
		{"boolean", `false,` + message},
		{"number", `7,` + message},
		{"string", `"discarded",` + message},
		{"array", `[],` + message},
		{"trailing-number", message + `,7`},
		{"prefix-null", `{"role":"user","content":"same request","prefix":null}`},
		{"prefix-false", `{"type":"message","role":"user","content":"same request","prefix":false}`},
		{"prefix-function", message + `,{"type":"function_call","call_id":"synthetic-call","name":"synthetic_lookup","arguments":"{}","prefix":false}`},
		{"prefix-text", `{"type":"input_text","text":"same request","prefix":null}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body := []byte(`{"input":[` + fixture.items + `]}`)
			_, _, err := normalizeGeminiChatInput043711(body)
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
				t.Fatalf("invalid input item became fresh upstream work: error=%v input=%s", err, body)
			}
		})
	}
	for _, items := range []string{
		message,
		`{"role":"user","content":"same request","suffix":null,"prompt":null,"metadata":{"prefix":null}}`,
	} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":[` + items + `]}`)); err != nil {
			t.Fatalf("valid input item rejected: %v", err)
		}
	}
}
