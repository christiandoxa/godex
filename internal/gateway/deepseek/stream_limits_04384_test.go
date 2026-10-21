package deepseek

import (
	"io"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekSSEEnforcesProviderWireLimits(t *testing.T) {
	const limit = 4 << 20
	const packet = `{"choices":[{"delta":{"content":"limit-control"}}]}`
	large := `{"unused":"` + strings.Repeat("a", 2<<20) + `","choices":[{"delta":{"content":"limit-control"}}]}`
	for _, tc := range []struct{ name, wire, failure string }{
		{"ordinary-control", "data: " + packet + "\n\ndata: [DONE]\n\n", ""},
		{"larger-than-one-MiB", "data: " + large + "\n\ndata: [DONE]\n\n", ""},
		{"oversized-comment", ":" + strings.Repeat("a", limit) + "\n\ndata: " + packet + "\n\ndata: [DONE]\n\n", "provider SSE line exceeded safe size limit (4194304)"},
		{"oversized-event", "data: \"" + strings.Repeat("a", 2<<20) + "\"\ndata: \"" + strings.Repeat("b", 2<<20) + "\"\n\ndata: [DONE]\n\n", "provider SSE event exceeded safe size limit (4194304)"},
		{"invalid-comment-UTF8", ":" + string([]byte{0xff}) + "\n\ndata: " + packet + "\n\ndata: [DONE]\n\n", "stream did not contain valid UTF-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(tc.wire)))
			if tc.failure != "" {
				if strings.Count(wire, "event: response.failed") != 1 || !strings.Contains(wire, `"message":"`+tc.failure+`"`) || strings.Contains(wire, "event: response.completed") || strings.Contains(wire, "limit-control") {
					t.Fatalf("wire limit was skipped or lost failure: %s", wire)
				}
			} else if !strings.Contains(wire, "limit-control") || strings.Count(wire, "event: response.completed") != 1 || strings.Contains(wire, "event: response.failed") {
				t.Fatalf("admissible wire lost content: %s", wire)
			}
		})
	}
}
