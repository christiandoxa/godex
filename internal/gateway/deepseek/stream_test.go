package deepseek

import (
	"strings"
	"testing"
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
