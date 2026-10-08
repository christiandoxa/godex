package deepseek

import (
	"strings"
	"testing"
	"time"
)

func TestTranslateDeepSeekSSEDataRejectsMalformedJSON(t *testing.T) {
	if _, _, err := translateDeepSeekSSEData([]byte("{bad")); err == nil {
		t.Fatal("malformed SSE JSON was accepted")
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
	state := newDeepSeekChatStreamState(7, nil, nil, deepSeekConversationStore{})
	state.createdAt = time.Unix(123, 0)
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
