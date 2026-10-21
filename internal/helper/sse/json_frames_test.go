package sse

import (
	"bytes"
	"strings"
	"testing"
)

func TestProdex04384SSEAccumulatedJSONHasExactBound(t *testing.T) {
	state := &JSONFrames{}
	first := []byte(`"` + strings.Repeat("a", accumulatedJSONMaxBytes-1))
	values, err := state.Observe(first)
	if err != nil || len(values) != 0 || len(state.pending) != accumulatedJSONMaxBytes {
		t.Fatalf("exact accumulation bound rejected: values=%d pending=%d err=%v", len(values), len(state.pending), err)
	}
	if _, err := state.Observe([]byte(`x`)); err == nil || err.Error() != "provider SSE accumulated JSON exceeded safe size limit (4194304)" || len(state.pending) != accumulatedJSONMaxBytes {
		t.Errorf("overflow changed bounded state or lost safe failure: pending=%d err=%v", len(state.pending), err)
	}
}

func TestJSONFramesPreservesCompleteAndSeparatePrecedence(t *testing.T) {
	state := new(JSONFrames)
	if values, err := state.Observe([]byte(`{"text":"frag`)); err != nil || len(values) != 0 || !state.Pending() {
		t.Fatalf("partial JSON failed or emitted: values=%q err=%v", values, err)
	}
	for _, packet := range []string{`{"complete":true}`, "null\n\n {\"a\":1}", "[0,\n1]"} {
		values, err := state.Observe([]byte(packet))
		if err != nil || len(values) == 0 || !state.Pending() {
			t.Fatalf("complete packet consumed pending fragment: packet=%q values=%q err=%v", packet, values, err)
		}
		if strings.HasPrefix(packet, "[") && (len(values) != 1 || string(values[0]) != packet) {
			t.Fatal("joined JSON lost precedence over separate lines")
		}
	}
	values, err := state.Observe([]byte(`ment"}`))
	if err != nil || len(values) != 1 || string(values[0]) != `{"text":"fragment"}` || state.Pending() {
		t.Fatalf("reassembly lost fragment: values=%q err=%v", values, err)
	}
}

func TestJSONFramesValidatesEverySeparateValueBeforeEmission(t *testing.T) {
	for _, invalid := range []string{`{bad`, `"\ud800"`, `1.7976931348623158e308`, `1e309`} {
		state := new(JSONFrames)
		values, err := state.Observe([]byte("{\"safe\":true}\n" + invalid))
		if err != nil || len(values) != 0 || !state.Pending() {
			t.Fatalf("invalid separate value emitted before complete validation: invalid=%s values=%q err=%v", invalid, values, err)
		}
	}
	state := new(JSONFrames)
	if values, err := state.Observe([]byte{0xff}); err == nil || len(values) != 0 || state.Pending() {
		t.Fatal("invalid UTF-8 was buffered or accepted")
	}
}

func TestJSONDoneRequiresValidUTF8AndChecksEveryDataLine(t *testing.T) {
	for _, packet := range []string{"[DONE]", "{bad\n [DONE] ", "[DONE]\n{bad"} {
		if !JSONDone([]byte(packet)) {
			t.Errorf("DONE marker missed: %q", packet)
		}
	}
	if JSONDone(bytes.Join([][]byte{{0xff}, []byte("[DONE]")}, []byte("\n"))) || JSONDone([]byte(`"[DONE]"`)) {
		t.Fatal("invalid UTF-8 or a JSON string authorized DONE")
	}
}
