package deepseek

import (
	"strings"
	"testing"
	"time"
)

// Source: tagged Prodex 0.438.0
// crates/prodex-mojo-core/src/rich/anthropic_messages.rs
// ANTHROPIC_RESPONSE_PLAN_MAX_BLOCKS = 65536. This input count is checked
// *before* classification, even for thinking blocks without any visible
// text or output. Do not silently accept an oversized response body.
func TestProdex04380AnthropicResponsePlanRejectsTooManyContentBlocks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		count  int
		reject bool
	}{
		{name: "exact bound", count: 65536},
		{name: "one past bound", count: 65537, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const block = `{"type":"thinking","thinking":false}`
			var source strings.Builder
			source.Grow(14 + tc.count*(len(block)+1))
			source.WriteString(`{"content":[`)
			for index := 0; index < tc.count; index++ {
				if index != 0 {
					source.WriteByte(',')
				}
				source.WriteString(block)
			}
			source.WriteString(`]}`)
			if source.Len() > 4<<20 {
				t.Fatal("fixture exceeded tagged 4 MiB bounded buffered-body gate")
			}
			result, err := deepSeekAnthropicResponse([]byte(source.String()), time.Unix(123, 0))
			if tc.reject {
				if err == nil || !strings.Contains(err.Error(), "Anthropic Messages response plan failed") {
					t.Fatalf("oversized Anthropic response accepted or error misclassified: err=%v bytes=%d", err, len(result))
				}
			} else if err != nil || !strings.Contains(string(result), `"output":[]`) {
				t.Fatalf("valid exact-bound content falsely rejected: err=%v body-prefix=%q", err, string(result)[:min(len(result), 100)])
			}
		})
	}
}
