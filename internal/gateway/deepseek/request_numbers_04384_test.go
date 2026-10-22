package deepseek

import (
	"strings"
	"testing"
)

func TestProdex04384DeepSeekRequestUsesCanonicalSerdeNumbersAndText(t *testing.T) {
	for _, fixture := range []struct{ name, fields, want string }{
		{"control", `"temperature":0.2,"top_p":0.8`, `"temperature":0.2,"top_p":0.8`},
		{"exponent", `"temperature":1e3`, `"temperature":1000.0`},
		{"negative_zero", `"top_p":-0`, `"top_p":-0.0`},
		{"fraction", `"temperature":0.123456789012345678901234567890`, `"temperature":0.12345678901234568`},
		{"unsigned", `"temperature":18446744073709551615`, `"temperature":18446744073709551615`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			body, err := ResponsesRequest([]byte(`{"model":"deepseek-v4-pro","input":"same request",`+fixture.fields+`}`), RequestOptions{})
			want := `{"messages":[{"content":"same request","role":"user"}],"model":"deepseek-v4-pro","stream":false,` + fixture.want + `}`
			if err != nil || string(body) != want {
				t.Fatalf("request differs from locked serde Value bytes: error=%v body=%s", err, body)
			}
		})
	}
	t.Run("unescaped_unicode", func(t *testing.T) {
		body, err := ResponsesRequest([]byte(`{"model":"deepseek-v4-pro","input":"<>&/\u2028\u2029"}`), RequestOptions{})
		if err != nil || !strings.Contains(string(body), "<>&/\u2028\u2029") {
			t.Fatalf("request text differs from serde Unicode bytes: error=%v body=%s", err, body)
		}
	})
}
