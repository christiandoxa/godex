package deepseek

import (
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestTranslateDeepSeekSSEDataRejectsRetainedMalformedJSONAtDone(t *testing.T) {
	state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
	if output, supported, err := state.observe([]byte("{bad")); err != nil || supported || len(output) != 0 {
		t.Fatalf("partial JSON was committed or rejected before DONE: output=%s error=%v", output, err)
	}
	output, _, err := state.observe([]byte("[DONE]"))
	if err != nil || !strings.Contains(string(output), `"message":"incomplete JSON segment"`) || strings.Contains(string(output), "response.completed") {
		t.Fatalf("malformed retained JSON was accepted at DONE: output=%s error=%v", output, err)
	}
}

func TestTranslateDeepSeekSSEDataAddsToolItemBeforeArgumentDelta(t *testing.T) {
	data := []byte(`{"choices":[{"delta":{"tool_calls":[{"id":"call_1","function":{"name":"lookup"}}]}}]}`)
	event, supported, err := translateDeepSeekSSEData(data)
	if err != nil || !supported || !strings.Contains(string(event), "response.output_item.added") ||
		!strings.Contains(string(event), `"call_id":"call_1"`) {
		t.Fatalf("tool call without argument delta = %q supported:%t err:%v", event, supported, err)
	}

	data = []byte(`{"choices":[{"delta":{"tool_calls":[]}}]}`)
	if event, supported, err := translateDeepSeekSSEData(data); err != nil || !supported ||
		!strings.Contains(string(event), "response.created") || strings.Contains(string(event), "response.output_text.delta") {
		t.Fatalf("empty tool-call delta = %q supported:%t err:%v", event, supported, err)
	}
}

func TestProdex04361DeepSeekStreamPreservesWhitespaceBytesAndTerminalOrder(t *testing.T) {
	state := newDeepSeekChatStreamState(7, nil, nil, chatcompat.ConversationStore{})
	state.createdAt = 123
	var stream strings.Builder
	for _, chunk := range []string{
		`{"id":"chatcmpl_empty_delta","choices":[{"delta":{"reasoning_content":"","refusal":"","content":""}}]}`,
		`{"id":"chatcmpl_empty_delta","choices":[{"delta":{"reasoning_content":" ","refusal":" ","content":" "}}]}`,
		`[DONE]`,
	} {
		event, emitted, err := state.observe([]byte(chunk))
		if err != nil || !emitted {
			t.Fatalf("chunk %s emitted:%t err:%v", chunk, emitted, err)
		}
		stream.Write(event)
	}
	output := stream.String()
	wantDelta := "event: response.output_text.delta\r\ndata: {\"created_at\":123,\"delta\":\" \",\"response_id\":\"chatcmpl_empty_delta\",\"sequence_number\":4,\"type\":\"response.output_text.delta\"}\r\n\r\n"
	if strings.Count(output, "event: response.output_text.delta\r\n") != 1 || !strings.Contains(output, wantDelta) || strings.Contains(output, `"delta":""`) {
		t.Fatalf("whitespace delta bytes = %q", output)
	}
	if delta, completed := strings.Index(output, wantDelta), strings.Index(output, "event: response.completed\r\n"); delta < 0 || completed <= delta {
		t.Fatalf("terminal event order = %q", output)
	}
}
