package deepseek

import (
	"io"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384DeepSeekSSEReassemblesJSONBeforeEmitting(t *testing.T) {
	state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
	first, last := `{"choices":[{"delta":{"content":"frag`, `ment"}}]}`
	if output, supported, err := state.observe([]byte(first)); err != nil || supported || len(output) != 0 {
		t.Fatalf("partial JSON failed or emitted before reassembly: err=%v output=%s", err, output)
	}
	var wire strings.Builder
	for _, value := range []string{`{"choices":[{"delta":{"content":"standalone"}}]}`, last, "[DONE]"} {
		output, _, err := state.observe([]byte(value))
		if err != nil {
			t.Fatal(err)
		}
		wire.Write(output)
	}
	output := wire.String()
	if strings.Count(output, `"delta":"standalone"`) != 1 || strings.Count(output, `"delta":"fragment"`) != 1 ||
		strings.Index(output, `"delta":"standalone"`) > strings.Index(output, `"delta":"fragment"`) ||
		strings.Count(output, "event: response.completed") != 1 || strings.Contains(output, "event: response.failed") {
		t.Fatalf("complete-value precedence/reassembly changed: %s", output)
	}
}

func TestProdex04384DeepSeekSSEDataLineAndFragmentContracts(t *testing.T) {
	const left = `{"choices":[{"delta":{"content":"left"}}]}`
	const right = `{"choices":[{"delta":{"content":"right"}}]}`
	for _, tc := range []struct {
		name, wire, text string
		failed           bool
	}{
		{"joined-control", "data: {\ndata: \"choices\":[{\"delta\":{\"content\":\"left\"}}]\ndata: }\n\ndata: [DONE]\n\n", "left", false},
		{"separate-values", "data: " + left + "\ndata: " + right + "\n\ndata: [DONE]\n\n", "leftright", false},
		{"fragments", "data: {\"choices\":[{\"delta\":{\"content\":\"frag\n\ndata: ment\"}}]}\n\ndata: [DONE]\n\n", "fragment", false},
		{"done-before-malformed", "data: {bad\ndata: [DONE]\n\n", "", false},
		{"null-value", "data: null\n\ndata: [DONE]\n\n", "", false},
		{"array-value", "data: []\n\ndata: [DONE]\n\n", "", false},
		{"invalid-second-control", "data: " + left + "\ndata: {bad\n\ndata: [DONE]\n\n", "", true},
		{"partial-eof", "data: {\"partial\":\n\n", "", true},
		{"partial-done", "data: {\"partial\":\n\ndata: [DONE]\n\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(tc.wire)))
			if tc.failed {
				if strings.Count(wire, "event: response.failed") != 1 || !strings.Contains(wire, `"message":"incomplete JSON segment"`) ||
					strings.Contains(wire, "event: response.completed") || strings.Contains(wire, "event: response.created") || strings.Contains(wire, "left") {
					t.Fatalf("invalid retained JSON leaked or lost exact failure: %s", wire)
				}
				return
			}
			if strings.Count(wire, "event: response.completed") != 1 || strings.Contains(wire, "event: response.failed") ||
				tc.text != "" && !strings.Contains(wire, `"text":"`+tc.text+`"`) {
				t.Fatalf("valid data framing lost completion/content: %s", wire)
			}
		})
	}
}
