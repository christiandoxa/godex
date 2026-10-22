package main

import (
	"net/http"
	"testing"
)

func TestProdex04384RawKeyIdentityOracleRejectsMutations(t *testing.T) {
	for _, phase := range []string{"deepseek-raw-key-id-limit", "deepseek-raw-key-id-over-limit", "deepseek-raw-key-id-unicode-limit", "deepseek-raw-key-id-unicode-over", "deepseek-raw-key-id-control", "deepseek-raw-key-id-mixed", "deepseek-raw-key-id-blank", "deepseek-raw-key-id-local-compact"} {
		fixture, known := rawKeyIdentityCase04384(phase)
		if !known {
			t.Fatal("unknown fixture")
		}
		run := productRun{Client: exchange{Status: 200, Body: rawKeyIdentityResponse04384,
			Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}}}
		if fixture.reject {
			run.ExitStatus, run.Client.Status = 2, 503
			run.Client.Body = `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}`
			run.Client.Headers.Set("Content-Type", "application/json")
		} else if fixture.compact {
			run.Client.Body = rawKeyCompactFixture04384
			run.Client.Headers = rawKeyLocalFixtureHeaders04384("resume-compact")
		} else {
			run.Upstream = []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "primary", Body: syntheticFixtureRequest}}
		}
		run.Stdout = run.Client.Body + "\n"
		if failures := rawKeyIdentityInvariants04384(scenarioResult{Name: phase, Runs: []productRun{run, run}}); len(failures) != 0 {
			t.Fatalf("valid identity fixture rejected: %v", failures)
		}
		for _, mutate := range []func(*productRun){
			func(r *productRun) { r.Client.Body = `{}` },
			func(r *productRun) { r.Client.Headers = nil },
			func(r *productRun) { r.Client.Status = 409 },
			func(r *productRun) { r.Retries++ },
			func(r *productRun) { r.StateIntegrity = []string{"corrupt"} },
			func(r *productRun) { r.Upstream = []upstreamRequest{{AuthOK: false}} },
		} {
			changed := run
			mutate(&changed)
			if failures := rawKeyIdentityInvariants04384(scenarioResult{Name: phase, Runs: []productRun{changed, changed}}); len(failures) == 0 {
				t.Fatalf("matching identity mutations escaped: %s", phase)
			}
		}
	}
}
