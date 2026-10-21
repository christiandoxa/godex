package deepseek

import (
	"strings"
	"testing"
)

// Prodex 0.438.4 provider-core translates native Anthropic Messages
// requests by deserializing the entire ChatCompletion input as a
// serde_json::Value before applying the Mojo converter. A malformed
// unknown nested field cannot silently disappear during Go conversion.
func TestProdex04384DeepSeekAnthropicMessagesRequestNeedsCompleteSerdeJSON(t *testing.T) {
	const prefix = `{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"synthetic hello"`
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"canonical", "", true},
		{"paired-surrogate", `,"ignored":"\ud83d\ude00"`, true},
		{"long-mantissa-valid", `,"ignored":1.79769313486231580e308`, true},
		{"unpaired-high", `,"ignored":"\ud800"`, false},
		{"unpaired-low", `,"ignored":"\udc00"`, false},
		{"invalid-utf8", `,"ignored":"` + string([]byte{0xff}) + `"`, false},
		{"near-f64-overflow", `,"ignored":1.7976931348623158e308`, false},
		{"obvious-overflow", `,"ignored":1e309`, false},
		{"overdepth", `,"ignored":` + strings.Repeat(`[`, 140) + `0` + strings.Repeat(`]`, 140), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(prefix + tc.extra + `}]}`)
			_, parseErr := parseAnthropicChatRequest(source)
			if accepted := parseErr == nil; accepted != tc.valid {
				t.Fatalf("invalid native Messages serde document accepted at parser=%t want=%t error=%v", accepted, tc.valid, parseErr)
			}
			translated, err := deepSeekAnthropicRequest(source)
			if accepted := err == nil; accepted != tc.valid {
				t.Fatalf("invalid native Messages serde document accepted=%t want=%t error=%v translated=%s", accepted, tc.valid, err, translated)
			}
			if tc.valid && !strings.Contains(string(translated), "synthetic hello") {
				t.Fatalf("valid request lost content: %s", translated)
			}
		})
	}
}
