package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384ToolSignatureOracleRejectsWhitespaceMutation(t *testing.T) {
	wire := canonicalToolSignatureWire04384()
	run := productRun{
		Name: "prodex", Client: exchange{Status: http.StatusOK, Body: wire, Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}},
		Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: strings.Replace(syntheticFixtureRequest, `"stream":false`, `"stream":true`, 1)}},
	}
	scenario := scenarioResult{Name: "sse-tool-signature", Runs: []productRun{run, run}}
	if failures := scenarioInvariants(scenario); len(failures) != 0 || !equivalentToolSignatureSSE04384(wire, wire) {
		t.Fatalf("canonical signature scenario rejected: %v", failures)
	}
	quoted, _ := json.Marshal(streamedToolSignature04384)
	for _, mutation := range []struct{ from, to string }{
		{string(quoted), `"sig_🧭"`}, {"gemini_thought_signature", "thought_signature"},
		{`"sequence_number":3`, `"sequence_number":4`}, {"call-toolargs", "other-call"},
	} {
		changed := strings.Replace(wire, mutation.from, mutation.to, 1)
		if changed == wire || equivalentToolSignatureSSE04384(changed, changed) {
			t.Fatalf("tool signature mutation escaped: %v", mutation)
		}
		for index := range scenario.Runs {
			scenario.Runs[index].Client.Body = changed
		}
		if failures := scenarioInvariants(scenario); len(failures) == 0 {
			t.Fatal("matching wrong signatures escaped independent assertions")
		}
	}
}
