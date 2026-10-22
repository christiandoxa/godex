package main

import (
	"net/http"
	"testing"
)

func TestProdex04384BufferedToolObjectOracleRejectsMutations(t *testing.T) {
	for _, phase := range []string{"deepseek-buffered-tool-key", "deepseek-buffered-tool-comma", "deepseek-buffered-tool-colon", "deepseek-buffered-tool-separator", "deepseek-buffered-tool-search-key", "deepseek-buffered-tool-padded-key"} {
		body := bufferedToolObjectResponse04384(phase)
		run := productRun{Stdout: body + "\n", Client: exchange{Status: 200, Body: body,
			Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}}
		if failures := bufferedToolObjectInvariants04384(scenarioResult{Name: phase, Runs: []productRun{run, run}}); len(failures) != 0 {
			t.Fatalf("valid buffered-tool fixture rejected: %v", failures)
		}
		for _, mutate := range []func(*productRun){
			func(r *productRun) { r.Client.Body = `{}` },
			func(r *productRun) { r.Stdout = "invalid" },
			func(r *productRun) { r.Client.Status = 502 },
			func(r *productRun) { r.ExitStatus = 2 },
			func(r *productRun) { r.Retries = 1 },
			func(r *productRun) { r.Upstream = nil },
			func(r *productRun) { r.StateIntegrity = []string{"corrupt"} },
		} {
			changed := run
			mutate(&changed)
			if failures := bufferedToolObjectInvariants04384(scenarioResult{Name: phase, Runs: []productRun{changed, changed}}); len(failures) == 0 {
				t.Fatalf("matching mutations escaped the oracle: %s", phase)
			}
		}
	}
}
