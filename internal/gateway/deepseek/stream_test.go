package deepseek

import "testing"

func TestTranslateDeepSeekSSEDataRejectsMalformedJSON(t *testing.T) {
	if _, _, err := translateDeepSeekSSEData([]byte("{bad")); err == nil {
		t.Fatal("malformed SSE JSON was accepted")
	}
}

func TestTranslateDeepSeekSSEDataSkipsToolCallWithoutArgumentDelta(t *testing.T) {
	data := []byte(`{"choices":[{"delta":{"tool_calls":[{"id":"call_1","function":{"name":"lookup"}}]}}]}`)
	if _, supported, err := translateDeepSeekSSEData(data); err != nil || supported {
		t.Fatalf("tool call without argument delta = supported:%t err:%v", supported, err)
	}

	data = []byte(`{"choices":[{"delta":{"tool_calls":[]}}]}`)
	if event, supported, err := translateDeepSeekSSEData(data); err != nil || !supported || string(event) != "event: response.output_text.delta\ndata: {\"delta\":\"\",\"type\":\"response.output_text.delta\"}\n\n" {
		t.Fatalf("empty tool-call delta = %q supported:%t err:%v", event, supported, err)
	}
}
