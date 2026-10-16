package deepseek

import (
	"strings"
	"testing"
	"time"
)

// Source: Prodex 0.438.0 mojo/prodex_core/anthropic_request.mojo
// ANTHROPIC_REQUEST_KERNEL_MAX_DEPTH = 128. A valid JSON tool payload can
// still exceed that parser bound, even when it is below the 4 MiB read cap.
func TestProdex04380AnthropicResponseToolInputDepthMustStayBounded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		depth  int
		reject bool
	}{
		{"shallow valid tool", 6, false},
		{"bounded nested tool", 64, false},
		{"exact 128-depth leaf", 126, false},
		{"129-depth leaf", 127, true},
		{"deep but short tool", 140, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			toolInput := strings.Repeat(`{"k":`, tc.depth) + `0` + strings.Repeat("}", tc.depth)
			payload := `{"content":[{"type":"tool_use","id":"call_synthetic","name":"lookup","input":` + toolInput + `}]}`
			if len(payload) > 4<<20 {
				t.Fatal("fixture unexpectedly larger than the tagged body limit")
			}
			converted, err := deepSeekAnthropicResponse([]byte(payload), time.Unix(123, 0))
			if tc.reject {
				if err == nil {
					t.Fatalf("over-deep Anthropic tool input was accepted: bytes=%d", len(converted))
				}
				return
			}
			if err != nil || !strings.Contains(string(converted), `"type":"function_call"`) {
				t.Fatalf("valid bounded tool input lost output: err=%v body=%q", err, converted)
			}
		})
	}
}

func TestProdex04380AnthropicJSONDepthIgnoresEscapedBraces(t *testing.T) {
	embedded := strings.Repeat("{[", 400) + strings.Repeat("]}", 400)
	encoded := strings.ReplaceAll(embedded, "\\", "\\\\")
	payload := `{"content":[{"type":"text","text":"` + encoded + `"}]}`
	result, err := deepSeekAnthropicResponse([]byte(payload), time.Unix(123, 0))
	if err != nil || !strings.Contains(string(result), "output_text") {
		t.Fatalf("braces inside valid string counted as nesting: err=%v", err)
	}
}

// The tagged translator projects only content to the Mojo kernel. An
// unrelated opaque metadata extension on the source response may be deeply
// structured without becoming an additional content-block input value.
func TestProdex04380AnthropicResponseDepthIgnoresOuterExtensions(t *testing.T) {
	metadata := strings.Repeat(`{"nested":`, 140) + `0` + strings.Repeat("}", 140)
	body := `{"content":[{"type":"text","text":"retain me"}],"vendor_extension":` + metadata + `}`
	result, err := deepSeekAnthropicResponse([]byte(body), time.Unix(123, 0))
	if err != nil || !strings.Contains(string(result), "retain me") {
		t.Fatalf("unprojected metadata extension incorrectly tripped depth limit: err=%v body=%q", err, result)
	}
}
