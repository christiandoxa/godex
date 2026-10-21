package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384StreamToolNameOracleRejectsTrimmedOrStaleNames(t *testing.T) {
	for _, name := range []string{"sse-tool-name-padded", "sse-tool-name-blank-namespace", "sse-tool-name-blank-late"} {
		wire := streamToolNameWire04384(name)
		run := productRun{
			Name: "prodex", Client: exchange{Status: http.StatusOK, Body: wire, Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: strings.Replace(syntheticFixtureRequest, `"stream":false`, `"stream":true`, 1)}},
		}
		scenario := scenarioResult{Name: name, Runs: []productRun{run, run}}
		if failures := scenarioInvariants(scenario); len(failures) != 0 || !equivalentStreamToolNameSSE04384(wire, wire) {
			t.Fatalf("canonical tool name trace rejected: %v", failures)
		}
		for _, mutate := range []struct{ old, replacement string }{
			{`"name":`, `"other_name":`}, {`"sequence_number":3`, `"sequence_number":4`},
			{"call-toolargs", "other-call"},
		} {
			changed := strings.Replace(wire, mutate.old, mutate.replacement, 1)
			if changed == wire || equivalentStreamToolNameSSE04384(changed, changed) {
				t.Fatal("tool name mutation escaped oracle")
			}
			for index := range scenario.Runs {
				scenario.Runs[index].Client.Body = changed
			}
			if failures := scenarioInvariants(scenario); len(failures) == 0 {
				t.Fatal("matching wrong tool name trace escaped independent assertions")
			}
		}
	}
}
