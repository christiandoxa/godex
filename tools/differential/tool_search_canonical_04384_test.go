package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384ToolSearchOracleRequiresExactCanonicalArgumentBytes(t *testing.T) {
	for _, stream := range []bool{false, true} {
		wire := canonicalToolSearchWire04384(stream)
		if !stream && !json.Valid([]byte(wire)) || stream && !equivalentCanonicalToolSearchSSE04384(wire, wire) {
			t.Fatal("canonical tool search fixture invalid")
		}
		name, request := "tool-search-canonical", syntheticFixtureRequest
		if stream {
			name = "sse-tool-search-canonical"
			request = strings.Replace(request, `"stream":false`, `"stream":true`, 1)
		}
		run := productRun{Name: "prodex", Client: exchange{Status: http.StatusOK, Body: wire}, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: request}}}
		scenario := scenarioResult{Name: name, Runs: []productRun{run, run}}
		if failures := scenarioInvariants(scenario); len(failures) != 0 {
			t.Fatalf("canonical tool search scenario rejected: %v", failures)
		}
		for _, mutation := range []struct{ old, replacement string }{
			{"9007199254740993", "9007199254740992"},
			{"18446744073709551615", "18446744073709551616"},
			{"100.0", "100"}, {"-0.0", "0"},
			{`"execution":"client"`, `"execution":"server"`},
			{`"type":"tool_search_call"`, `"type":"function_call"`},
		} {
			changed := strings.Replace(wire, mutation.old, mutation.replacement, 1)
			if changed == wire || stream && equivalentCanonicalToolSearchSSE04384(wire, changed) {
				t.Fatal("canonical tool search oracle accepted changed argument bytes")
			}
			scenario.Runs[1].Client.Body = changed
			if failures := scenarioInvariants(scenario); len(failures) == 0 {
				t.Fatal("matching wrong tool-search output escaped independent assertions")
			}
		}
	}
}
