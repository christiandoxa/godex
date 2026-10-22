package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384BufferedViewOracleRejectsMatchingWireMutations(t *testing.T) {
	for _, phase := range bufferedFragmentCases04384() {
		body := bufferedFragmentResponse04384(phase)
		run := productRun{Stdout: body + "\n", Client: exchange{Status: 200, Body: body,
			Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}}
		if failures := scenarioInvariants(scenarioResult{Name: phase, Runs: []productRun{run, run}}); len(failures) != 0 {
			t.Fatalf("valid buffered view rejected: %v", failures)
		}
		for _, mutate := range []func(*productRun){
			func(r *productRun) { r.Client.Body = `{}` },
			func(r *productRun) {
				r.Client.Body = strings.Replace(r.Client.Body, "chatcmpl-differential-tool", "response-changed", 1)
			},
			func(r *productRun) { r.Stdout = "changed" },
			func(r *productRun) { r.Retries = 1 },
			func(r *productRun) { r.Upstream = nil },
		} {
			changed := run
			mutate(&changed)
			if failures := scenarioInvariants(scenarioResult{Name: phase, Runs: []productRun{changed, changed}}); len(failures) == 0 {
				t.Fatalf("matching wire mutation escaped: %s", phase)
			}
		}
	}
}
