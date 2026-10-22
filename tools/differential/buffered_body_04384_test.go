package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProdex04384BufferedBodyFixtureExactSizeAndContent(t *testing.T) {
	const original = `{"choices":[{"message":{"content":"café","tool_calls":[]}}],"id":"fixture"}`
	for _, phase := range bufferedBodyCases04384() {
		body := bufferedBodyUpstream04384(phase, original)
		size, _ := bufferedBodyCase04384(phase)
		if len(body) != size {
			t.Fatalf("%s body bytes = %d, want %d", phase, len(body), size)
		}
		var root map[string]any
		if err := json.Unmarshal([]byte(body), &root); err != nil {
			t.Fatal(err)
		}
		delete(root, "padding")
		encoded, err := json.Marshal(root)
		if err != nil || string(encoded) != original {
			t.Fatalf("%s changed original payload: %v", phase, err)
		}
	}
}

func TestProdex04384BufferedBodyOracleRejectsMatchingMutations(t *testing.T) {
	for _, phase := range bufferedBodyCases04384() {
		body := bufferedBodyExpected04384(phase)
		run := productRun{Stdout: body + "\n", Client: exchange{Status: 200, Body: body,
			Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}}
		if size, _ := bufferedBodyCase04384(phase); size > 4<<20 {
			run.ExitStatus, run.Client.Status = 2, 502
			run.Client.Headers.Set("Content-Type", "text/plain; charset=utf-8")
		}
		if phase == "deepseek-buffered-body-over-rotation" {
			run.Upstream[0].KeySlot = "primary"
		}
		if failures := scenarioInvariants(scenarioResult{Name: phase, Runs: []productRun{run, run}}); len(failures) != 0 {
			t.Fatalf("valid body boundary control rejected: %v", failures)
		}
		for _, mutate := range []func(*productRun){
			func(r *productRun) { r.Client.Body = `{}` },
			func(r *productRun) { r.Client.Status++ },
			func(r *productRun) { r.ExitStatus++ },
			func(r *productRun) { r.Client.Headers.Set("Content-Type", "text/plain") },
			func(r *productRun) { r.Stdout = "changed" },
			func(r *productRun) { r.StateIntegrity = []string{"changed"} },
			func(r *productRun) { r.Retries = 1 },
			func(r *productRun) { r.Upstream = nil },
			func(r *productRun) { r.Upstream[0].AuthOK = false },
			func(r *productRun) { r.Upstream[0].KeySlot = "next" },
			func(r *productRun) { r.Upstream[0].Body = `{}` },
		} {
			changed := run
			changed.Client.Headers = run.Client.Headers.Clone()
			changed.Upstream = append([]upstreamRequest(nil), run.Upstream...)
			mutate(&changed)
			if failures := scenarioInvariants(scenarioResult{Name: phase, Runs: []productRun{changed, changed}}); len(failures) == 0 {
				t.Fatalf("equal body-boundary corruption escaped: %s", phase)
			}
		}
	}
}
