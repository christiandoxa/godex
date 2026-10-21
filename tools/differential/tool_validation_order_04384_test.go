package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384ToolValidationOrderOracleRejectsMutations(t *testing.T) {
	wire := toolValidationOrderWire04384()
	if !equivalentToolValidationOrderSSE04384(wire, wire) || strings.Count(wire, "event: ") != 36 {
		t.Fatal("ordered tool validation fixture rejected")
	}
	for _, name := range []string{"sse-tool-order-finish", "sse-tool-order-done"} {
		run := productRun{
			Name: "prodex", Client: exchange{Status: http.StatusOK, Body: wire, Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: strings.Replace(syntheticFixtureRequest, `"stream":false`, `"stream":true`, 1)}},
		}
		scenario := scenarioResult{Name: name, Runs: []productRun{run, run}}
		if failures := scenarioInvariants(scenario); len(failures) != 0 {
			t.Fatalf("canonical scenario rejected: %v", failures)
		}
		for _, mutation := range []struct{ from, to string }{
			{"at index 3:", "at index 18:"},
			{`"sequence_number":35`, `"sequence_number":36`},
			{`"output_index":18`, `"output_index":3`},
			{"response.failed", "response.completed"},
			{"invalid_tool_call_arguments", "provider_stream_error"},
		} {
			changed := strings.Replace(wire, mutation.from, mutation.to, 1)
			if changed == wire || equivalentToolValidationOrderSSE04384(wire, changed) || equivalentToolValidationOrderSSE04384(changed, changed) {
				t.Fatalf("mutation escaped full-wire contract: %v", mutation)
			}
			for index := range scenario.Runs {
				scenario.Runs[index].Client.Body = changed
			}
			if failures := scenarioInvariants(scenario); len(failures) == 0 {
				t.Fatal("matching wrong tool diagnostics escaped independent assertions")
			}
		}
	}
}
