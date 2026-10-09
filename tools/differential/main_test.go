package main

import (
	"runtime/debug"
	"testing"
)

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

func TestGodexBuildSettingsMatchExactCleanSource(t *testing.T) {
	const expected = "382c2a1f171568b4c43296115a75cdc34ac1ee5c"
	valid := []debug.BuildSetting{
		{Key: "vcs.revision", Value: expected},
		{Key: "vcs.modified", Value: "false"},
	}
	if err := verifyGodexBuildSettings(valid, expected); err != nil {
		t.Fatalf("clean build rejected: %v", err)
	}
	for _, test := range []struct {
		name     string
		settings []debug.BuildSetting
	}{
		{"dirty", []debug.BuildSetting{{Key: "vcs.revision", Value: expected}, {Key: "vcs.modified", Value: "true"}}},
		{"stale", []debug.BuildSetting{{Key: "vcs.revision", Value: "4bf54d91d14942948bbe91e1f9f23cea7a947275"}, {Key: "vcs.modified", Value: "false"}}},
		{"no_buildinfo", nil},
		{"unknown_dirty_flag", []debug.BuildSetting{{Key: "vcs.revision", Value: expected}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := verifyGodexBuildSettings(test.settings, expected); err == nil {
				t.Fatal("invalid binary source provenance accepted")
			}
		})
	}
}
