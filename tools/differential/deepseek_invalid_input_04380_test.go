package main

import (
	"net/http"
	"testing"
)

func TestProdex04380DeepSeekInvalidInputMutationControls(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(productRun) productRun
		want   string
	}{
		{"success", func(run productRun) productRun {
			run.Client.Status = http.StatusOK
			run.Client.Body = `{"object":"response"}`
			return run
		}, ".local_error"},
		{"status", func(run productRun) productRun {
			run.Client.Status = http.StatusBadRequest
			return run
		}, ".local_error"},
		{"body", func(run productRun) productRun {
			run.Client.Body = "accepted"
			return run
		}, ".local_error"},
		{"content_type", func(run productRun) productRun {
			run.Client.Headers = run.Client.Headers.Clone()
			run.Client.Headers.Set("Content-Type", "application/json")
			return run
		}, ".error_content_type"},
		{"exit", func(run productRun) productRun {
			run.ExitStatus = 0
			return run
		}, ".exit_status"},
		{"attempt", func(run productRun) productRun {
			run.Upstream = []upstreamRequest{{Method: http.MethodPost}}
			return run
		}, ".upstream_attempts"},
		{"retry", func(run productRun) productRun {
			run.Retries = 1
			return run
		}, ".upstream_attempts"},
		{"event", func(run productRun) productRun {
			run.Events = []upstreamEvent{{Kind: "upstream.request", Attempt: 1}}
			return run
		}, ".upstream_attempts"},
	}
	for _, name := range deepSeekInvalidInputScenarioNames() {
		baseline := productRun{
			ExitStatus: 2,
			Client: exchange{
				Status:  http.StatusBadGateway,
				Headers: http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
				Body:    deepSeekLocalError,
			},
		}
		if failures := scenarioInvariants(scenarioResult{Name: name, Runs: []productRun{baseline, baseline}}); len(failures) != 0 {
			t.Fatalf("canonical invalid-input fixture %q rejected: %v", name, failures)
		}
		for _, mutation := range mutations {
			t.Run(name+"/"+mutation.name, func(t *testing.T) {
				scenario := scenarioResult{Name: name, Runs: []productRun{baseline, baseline}}
				scenario.Runs[1] = mutation.mutate(scenario.Runs[1])
				for _, failure := range scenarioInvariants(scenario) {
					if len(failure) >= len(mutation.want) && failure[len(failure)-len(mutation.want):] == mutation.want {
						return
					}
				}
				t.Fatalf("mutation escaped invariants: failures=%v", scenarioInvariants(scenario))
			})
		}
	}
}

func TestProdex04380DeepSeekInvalidInputFixtures(t *testing.T) {
	oversized := deepSeekInvalidInputFixture("deepseek-request-over-kernel-limit")
	if len(oversized) <= 4<<20 {
		t.Fatalf("oversized fixture length = %d, want > %d", len(oversized), 4<<20)
	}
	for _, name := range deepSeekInvalidInputScenarioNames() {
		if !isKnownDeepSeekInvalidInput(name) || deepSeekInvalidInputFixture(name) == nil {
			t.Fatalf("fixture %q is not available", name)
		}
	}
}
