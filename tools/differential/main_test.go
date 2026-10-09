package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"
)

const (
	syntheticFixtureRequest  = `{"model":"deepseek-v4-pro","stream":false,"messages":[{"role":"user","content":"same request"}]}`
	syntheticFixtureResponse = `{"object":"response","model":"deepseek-v4-pro","output":[{"type":"message","content":[{"type":"output_text","text":"synthetic-ok"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
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
		{Name: "prodex", ExitStatus: 1, Cancelled: true, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, Body: syntheticFixtureRequest}}},
		{Name: "godex", ExitStatus: 1, Cancelled: true, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, Body: syntheticFixtureRequest}}},
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

func TestCleanCanonicalSourceCheckRejectsNewFiles(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("init isolated source repo: %v: %s", err, out)
	}
	if err := requireCleanSource(root); err != nil {
		t.Fatalf("clean source rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "unexpected.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireCleanSource(root); err == nil {
		t.Fatal("untracked source change was accepted as canonical")
	}
}

// The real Prodex 0.436.1 single-key 429 path does NOT retry in the proxy.
func TestSingleKey429RejectsUnownedProxyRetries(t *testing.T) {
	runs := []productRun{
		{Name: "prodex", ExitStatus: 2, Client: exchange{Status: 429, Body: "rate_limit_exceeded"}, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, Body: syntheticFixtureRequest}}},
		{Name: "godex", ExitStatus: 2, Client: exchange{Status: 429, Body: "rate_limit_exceeded"}, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, Body: syntheticFixtureRequest}}},
	}
	if failures := scenarioInvariants(scenarioResult{Name: "single-key-429", Runs: runs}); len(failures) != 0 {
		t.Fatalf("canonical terminal rate-limit rejected: %v", failures)
	}
	runs[1].Upstream = append(runs[1].Upstream, upstreamRequest{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, Body: syntheticFixtureRequest})
	runs[1].Retries = 1
	if failures := scenarioInvariants(scenarioResult{Name: "single-key-429", Runs: runs}); len(failures) == 0 {
		t.Fatal("unexpected proxy retry was incorrectly accepted")
	}
}

// Two products can match on an invalid credential after their Authorization
// bytes are redacted. The independent oracle must reject that false PASS.
func TestScenarioInvariantsDetectsTwoMatchingWrongCredentials(t *testing.T) {
	invalid := productRun{
		ExitStatus: 0,
		Client:     exchange{Status: 200, Body: syntheticFixtureResponse},
		Upstream:   []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: false, Body: syntheticFixtureRequest}},
	}
	if differences := compare(invalid, invalid); len(differences) != 0 {
		t.Fatalf("fixture should compare equal before oracle: %v", differences)
	}
	if failures := scenarioInvariants(scenarioResult{Name: "success", Runs: []productRun{invalid, invalid}}); len(failures) == 0 {
		t.Fatal("matching invalid Authorization headers escaped the independent oracle")
	}
}

func TestFixtureOracleRejectsSymmetricCorruption(t *testing.T) {
	if !validFixtureRequest(syntheticFixtureRequest) || !validFixtureResponse(syntheticFixtureResponse) {
		t.Fatal("canonical fixture rejected")
	}
	if validFixtureRequest(`{"model":"wrong","stream":false,"messages":[{"role":"user","content":"same request"}]}`) {
		t.Fatal("wrong model accepted")
	}
	if validFixtureRequest(`{"model":"deepseek-v4-pro","stream":true,"messages":[{"role":"user","content":"same request"}]}`) {
		t.Fatal("wrong stream mode accepted")
	}
	if validFixtureResponse(`{"object":"response","model":"deepseek-v4-pro","output":[{"type":"message","content":[{"type":"output_text","text":"wrong"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`) {
		t.Fatal("incorrect assistant response accepted")
	}
	if validFixtureResponse(`{"object":"response","model":"deepseek-v4-pro","output":[{"type":"message","content":[{"type":"output_text","text":"synthetic-ok"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":400}}`) {
		t.Fatal("incorrect usage accepted")
	}
}
