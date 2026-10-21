package gemini

import (
	"strings"
	"testing"
)

// The 0.438.4 Gemini translator parses a complete serde_json::Value before
// extracting the model, input history or tools. Even unrelated invalid
// fields make the entire request invalid and may not reach either native
// GenerateContent or the chat-compatible API-key bridge.
func TestProdex04384GeminiRequestRequiresCompleteSerdeJSON(t *testing.T) {
	const prefix = `{"model":"gemini-3.1-pro-preview","input":[{"role":"user","content":"hello"}]`
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"canonical", "", true},
		{"valid-unicode-pair", `,"unused":"\ud83d\ude00"`, true},
		{"valid-near-maximum-mantissa", `,"unused":1.79769313486231580e308`, true},
		{"lone-high-surrogate", `,"unused":"\ud800"`, false},
		{"lone-low-surrogate", `,"unused":"\udc00"`, false},
		{"invalid-UTF8", `,"unused":"` + string([]byte{0xff}) + `"`, false},
		{"serde-near-max-f64-overflow", `,"unused":1.7976931348623158e308`, false},
		{"unbounded-number", `,"unused":1e309`, false},
		{"excess-nesting", `,"unused":` + strings.Repeat(`[`, 140) + `0` + strings.Repeat(`]`, 140), false},
	} {
		body := []byte(prefix + tc.extra + `}`)
		for _, chat := range []bool{false, true} {
			label := tc.name + "/native"
			if chat {
				label = tc.name + "/chat-compatible"
			}
			t.Run(label, func(t *testing.T) {
				if chat {
					result, err := normalizeGeminiChatInput043711(body)
					if accepted := err == nil; accepted != tc.valid {
						t.Fatalf("serde invalid Gemini Chat request accepted=%t want=%t error=%v output=%q", accepted, tc.valid, err, result)
					}
					return
				}
				result, err := translateResponsesRequest(body, "gemini-3.1-pro-preview")
				if accepted := err == nil; accepted != tc.valid {
					t.Fatalf("serde invalid Gemini native request accepted=%t want=%t error=%v output=%q", accepted, tc.valid, err, result.body)
				}
				if tc.valid && len(result.body) == 0 {
					t.Fatal("valid Gemini request lost GenerateContent body")
				}
			})
		}
	}
}
