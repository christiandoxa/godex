package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384StreamToolIDOracleRejectsChangedIdentities(t *testing.T) {
	for _, name := range []string{"sse-tool-id-missing", "sse-tool-id-empty", "sse-tool-id-padded"} {
		wire := streamToolIDWire04384(name)
		if name == "sse-tool-id-missing" {
			wire = strings.ReplaceAll(wire, "call_deepseek_fixture", "call_deepseek_01900000-0000-7000-8000-000000000001")
		}
		run := productRun{
			Name: "prodex", Client: exchange{Status: http.StatusOK, Body: wire, Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: strings.Replace(syntheticFixtureRequest, `"stream":false`, `"stream":true`, 1)}},
		}
		scenario := scenarioResult{Name: name, Runs: []productRun{run, run}}
		if failures := scenarioInvariants(scenario); len(failures) != 0 || !equivalentStreamToolIDSSE04384(wire, wire) {
			t.Fatalf("canonical tool ID scenario rejected: %v", failures)
		}
		for _, changed := range []string{
			strings.Replace(wire, `"call_id":`, `"other_id":`, 1),
			strings.Replace(wire, `"sequence_number":3`, `"sequence_number":4`, 1),
			strings.Replace(wire, `"type":"function_call"`, `"type":"custom_tool_call"`, 1),
			strings.Replace(wire, `"call_id":`, `"call_id":"wrong","unused":`, 1),
		} {
			if changed == wire || equivalentStreamToolIDSSE04384(changed, changed) {
				t.Fatalf("changed tool ID trace accepted: %s", name)
			}
			for index := range scenario.Runs {
				scenario.Runs[index].Client.Body = changed
			}
			if failures := scenarioInvariants(scenario); len(failures) == 0 {
				t.Fatal("matching wrong tool IDs escaped independent assertions")
			}
		}
	}
}
