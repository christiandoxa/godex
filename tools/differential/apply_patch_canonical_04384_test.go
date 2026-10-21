package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384ApplyPatchOracleRejectsNumericAndEventMutations(t *testing.T) {
	wire := canonicalApplyPatchWire04384()
	run := productRun{
		Name: "prodex", Client: exchange{Status: http.StatusOK, Body: wire, Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}},
		Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: strings.Replace(syntheticFixtureRequest, `"stream":false`, `"stream":true`, 1)}},
	}
	scenario := scenarioResult{Name: "sse-apply-patch-canonical", Runs: []productRun{run, run}}
	if failures := scenarioInvariants(scenario); len(failures) != 0 || !equivalentCanonicalApplyPatchSSE04384(wire, wire) {
		t.Fatalf("canonical apply-patch scenario rejected: %v", failures)
	}
	for _, mutation := range []struct{ from, to string }{
		{"9007199254740993", "9007199254740992"}, {"18446744073709551615", "18446744073709552000"},
		{"100.0", "100"}, {"-0.0", "-0"}, {`"sequence_number":2`, `"sequence_number":3`},
		{`"input":""`, `"input":"unexpected"`}, {"custom_tool_call", "function_call"},
	} {
		changed := strings.Replace(wire, mutation.from, mutation.to, 1)
		if changed == wire || equivalentCanonicalApplyPatchSSE04384(wire, changed) || equivalentCanonicalApplyPatchSSE04384(changed, changed) {
			t.Fatalf("mutation escaped apply-patch contract: %v", mutation)
		}
		for index := range scenario.Runs {
			scenario.Runs[index].Client.Body = changed
		}
		if failures := scenarioInvariants(scenario); len(failures) == 0 {
			t.Fatal("matching wrong apply-patch output escaped independent assertions")
		}
	}
}
