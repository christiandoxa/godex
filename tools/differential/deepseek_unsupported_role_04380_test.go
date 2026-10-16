package main

import (
	"net/http"
	"testing"
)

func TestProdex04380UnsupportedMessageRoleMutationControls(t *testing.T) {
	baseline := productRun{
		ExitStatus: 2,
		Client: exchange{
			Status:  http.StatusBadGateway,
			Headers: http.Header{"Content-Type": {"text/plain; charset=utf-8"}},
			Body:    deepSeekUnsupportedRoleError,
		},
	}
	scenario := scenarioResult{
		Name: "deepseek-unsupported-message-role",
		Runs: []productRun{baseline, baseline},
	}
	if failures := scenarioInvariants(scenario); len(failures) != 0 {
		t.Fatalf("canonical local rejection rejected: %v", failures)
	}
	mutations := []struct {
		name   string
		mutate func(productRun) productRun
		want   string
	}{
		{"status", func(run productRun) productRun { run.Client.Status = http.StatusBadRequest; return run }, ".local_error"},
		{"error", func(run productRun) productRun { run.Client.Body = "accepted"; return run }, ".local_error"},
		{"exit", func(run productRun) productRun { run.ExitStatus = 0; return run }, ".exit_status"},
		{"upstream", func(run productRun) productRun {
			run.Upstream = []upstreamRequest{{Method: http.MethodPost}}
			return run
		}, ".upstream_attempts"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := scenario
			mutated.Runs = append([]productRun(nil), scenario.Runs...)
			mutated.Runs[1] = mutation.mutate(mutated.Runs[1])
			failures := scenarioInvariants(mutated)
			for _, failure := range failures {
				if len(failure) >= len(mutation.want) && failure[len(failure)-len(mutation.want):] == mutation.want {
					return
				}
			}
			t.Fatalf("mutation escaped invariants: failures=%v", failures)
		})
	}
}
