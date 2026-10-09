package main

import "testing"

func TestNegativeControlDetectsClientStatusMismatch(t *testing.T) {
	if !negativeControl() {
		t.Fatal("comparison did not detect changed client status")
	}
}

func TestRedactRemovesSyntheticKey(t *testing.T) {
	if got := redact("Authorization: Bearer " + apiKey); got != "Authorization: Bearer <synthetic-key>" {
		t.Fatalf("redact = %q", got)
	}
}

func TestScenarioInvariantsRejectEquivalentFailedRuns(t *testing.T) {
	scenario := scenarioResult{Name: "success", Runs: []productRun{
		{Name: "prodex", ExitStatus: 1},
		{Name: "godex", ExitStatus: 1},
	}}
	if len(scenarioInvariants(scenario)) == 0 {
		t.Fatal("matching failures must not satisfy the success contract")
	}
}

func TestScenarioInvariantsAllowExpectedInterruptedExit(t *testing.T) {
	scenario := scenarioResult{Name: "cancel", Runs: []productRun{
		{Name: "prodex", ExitStatus: 1, Cancelled: true, Upstream: []upstreamRequest{{Method: "POST"}}},
		{Name: "godex", ExitStatus: 1, Cancelled: true, Upstream: []upstreamRequest{{Method: "POST"}}},
	}}
	if failures := scenarioInvariants(scenario); len(failures) != 0 {
		t.Fatalf("expected interrupted client outcome rejected: %v", failures)
	}
	scenario.Runs[1].Cancelled = false
	if failures := scenarioInvariants(scenario); len(failures) == 0 {
		t.Fatal("missing cancellation must be rejected")
	}
}

func TestComparisonRejectsDifferentResponseBody(t *testing.T) {
	a := productRun{Client: exchange{Status: 200, Body: "correct"}}
	b := productRun{Client: exchange{Status: 200, Body: "incorrect"}}
	if differences := compare(a, b); !contains(differences, "client.body") {
		t.Fatalf("body mismatch was not detected: %v", differences)
	}
}
