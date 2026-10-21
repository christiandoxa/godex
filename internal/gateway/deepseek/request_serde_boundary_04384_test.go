package deepseek

import (
	"strings"
	"testing"
)

// Published 0.438.4 DeepSeek request_transform.rs calls
// serde_json::from_slice::<Value> before inspecting model/history/tools.
// An ignored invalid field cannot turn into a valid provider request merely
// because Go json.Decoder tolerates malformed UTF-8 or near-max floats.
func TestProdex04384DeepSeekResponsesRequestRequiresCompleteSerdeJSON(t *testing.T) {
	const prefix = `{"model":"deepseek-v4-pro","input":[{"role":"user","content":"synthetic text"}]`
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"canonical", "", true},
		{"valid-surrogate-pair", `,"unused":"\ud83d\ude00"`, true},
		{"valid-long-mantissa", `,"unused":1.79769313486231580e308`, true},
		{"lone-high-surrogate", `,"unused":"\ud800"`, false},
		{"lone-low-surrogate", `,"unused":"\udc00"`, false},
		{"invalid-UTF8", `,"unused":"` + string([]byte{0xff}) + `"`, false},
		{"near-max-serde-f64-overflow", `,"unused":1.7976931348623158e308`, false},
		{"out-of-range-exponent", `,"unused":1e309`, false},
		{"overdepth", `,"unused":` + strings.Repeat(`[`, 140) + `0` + strings.Repeat(`]`, 140), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(prefix + tc.extra + `}`)
			result, err := TranslateResponsesRequest(body, RequestOptions{Model: "deepseek-v4-pro"})
			if accepted := err == nil; accepted != tc.valid {
				t.Fatalf("serde-invalid DeepSeek request accepted=%t want=%t err=%v upstream=%q", accepted, tc.valid, err, result.Body)
			}
			if tc.valid && len(result.Body) == 0 {
				t.Fatal("valid DeepSeek request lost translated body")
			}
			// History replay uses the same lower-level parser and may not extract
			// user/tool context from a document that official serde rejects.
			_, err = parseResponsesObjectWithoutValidation(body)
			if accepted := err == nil; accepted != tc.valid {
				t.Fatalf("invalid serde JSON accepted by history parser=%t want=%t err=%v", accepted, tc.valid, err)
			}
		})
	}
}
