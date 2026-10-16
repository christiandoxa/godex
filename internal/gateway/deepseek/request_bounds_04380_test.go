package deepseek

import (
	"strings"
	"testing"
)

// The tagged request_policy.rs bounds serde_json::to_string(value), not raw input.
func TestProdex04380DeepSeekRequestBoundUsesNormalizedJSON(t *testing.T) {
	for _, fixture := range []struct{ name, body string }{
		{"whitespace", strings.Repeat(" ", 4<<20) + `{"input":"valid"}`},
		{"duplicate-key", `{"padding":"` + strings.Repeat("x", 4<<20) + `","padding":"","input":"valid"}`},
		{"escaped-text", `{"input":"` + strings.Repeat(`\u0078`, (4<<20)/6+1) + `"}`},
		{"html-text", `{"input":"` + strings.Repeat("<>&", (4<<20)/6) + `"}`},
		{"unicode-lines", `{"input":"` + strings.Repeat("\u2028\u2029", (4<<20)/6-32) + `"}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if _, err := ResponsesRequest([]byte(fixture.body), RequestOptions{}); err != nil {
				t.Fatalf("valid normalized request rejected: %v", err)
			}
		})
	}
}
