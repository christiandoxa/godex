package main

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestProdex04384ScenarioCompletionChecksTimeBoundSSEInvariant(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		offset time.Duration
	}{
		{"fresh", 0},
		{"stale", -10 * time.Minute},
		{"future", 10 * time.Minute},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			wire := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"Please try again in 1s.\"},\"id\":\"resp_deepseek_01a11f94-b55e-73ce-ac35-d0f0438a99f3\"},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", time.Now().Add(fixture.offset).Unix())
			run := productRun{Name: "prodex", Client: exchange{Status: 200, Body: wire,
				Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}},
				Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "primary", Body: syntheticFixtureRequest}}}
			candidate := run
			candidate.Name = "godex"
			spec := differentialScenario043713{name: "deepseek-sse-terminal", run: func(*mockServer) (scenarioResult, error) {
				return scenarioResult{Name: "deepseek-sse-terminal", Runs: []productRun{run, candidate}}, nil
			}}
			results, err := executeDifferentialScenarios043713([]differentialScenario043713{spec}, spec.name, 1)
			if err != nil || len(results) != 1 {
				t.Fatalf("scenario execution failed: %v", err)
			}
			want := 0
			if fixture.offset != 0 {
				want = 2
			}
			if len(results[0].Comparison) != want {
				t.Fatalf("scenario completion lost its timestamp evidence: differences=%v want=%d", results[0].Comparison, want)
			}
			if want != 0 {
				for _, name := range []string{"prodex", "godex"} {
					if !slices.Contains(results[0].Comparison, "deepseek-sse-terminal."+name+".embedded_stream_error") {
						t.Fatal("scenario completion lost the exact timestamp invariant")
					}
				}
			}
		})
	}
}
