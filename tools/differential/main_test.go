package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
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
		{Name: "prodex", ExitStatus: 1, Cancelled: true, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}},
		{Name: "godex", ExitStatus: 1, Cancelled: true, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}},
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
		{Name: "prodex", ExitStatus: 2, Client: exchange{Status: 429, Body: "rate_limit_exceeded"}, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}},
		{Name: "godex", ExitStatus: 2, Client: exchange{Status: 429, Body: "rate_limit_exceeded"}, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}},
	}
	if failures := scenarioInvariants(scenarioResult{Name: "single-key-429", Runs: runs}); len(failures) != 0 {
		t.Fatalf("canonical terminal rate-limit rejected: %v", failures)
	}
	runs[1].Upstream = append(runs[1].Upstream, upstreamRequest{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest})
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

// A rate-limited first run is terminal, but fresh traffic after restart
// must use the same configured provider successfully, without backoff poisoning.
func TestRecoverAfter429RequiresHealthySecondGeneration(t *testing.T) {
	bad := productRun{
		Name: "prodex", ExitStatus: 2, Client: exchange{Status: 429, Body: "rate_limit_exceeded"},
		Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}},
	}
	good := productRun{
		Name: "prodex", ExitStatus: 0, Client: exchange{Status: 200, Body: syntheticFixtureResponse},
		Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}},
	}
	scenario := scenarioResult{Name: "recover-after-429", Runs: []productRun{bad, good, bad, good}}
	if failures := scenarioInvariants(scenario); len(failures) != 0 {
		t.Fatalf("valid process recovery rejected: %v", failures)
	}
	scenario.Runs[3] = bad
	if failures := scenarioInvariants(scenario); len(failures) == 0 {
		t.Fatal("restart that remains rate-limited incorrectly passed")
	}
}

func TestSingleKey503RequiresOriginalServiceStatus(t *testing.T) {
	runs := []productRun{
		{Name: "prodex", ExitStatus: 2, Client: exchange{Status: 503, Body: "rate_limit_exceeded"},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}},
		{Name: "godex", ExitStatus: 2, Client: exchange{Status: 503, Body: "rate_limit_exceeded"},
			Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}}},
	}
	if failures := scenarioInvariants(scenarioResult{Name: "single-key-503", Runs: runs}); len(failures) != 0 {
		t.Fatalf("canonical single-key service outage rejected: %v", failures)
	}
	runs[1].Client.Status = 200
	if failures := scenarioInvariants(scenarioResult{Name: "single-key-503", Runs: runs}); len(failures) == 0 {
		t.Fatal("silently healed provider outage was accepted")
	}
}

// The terminal 503 must not prevent a healthy retry on a later process after
// provider health state is loaded from the same isolated persistent home.
func TestRecoverAfter503RequiresHealthySecondGeneration(t *testing.T) {
	outage := productRun{
		Name: "prodex", ExitStatus: 2,
		Client:   exchange{Status: 503, Body: "rate_limit_exceeded"},
		Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}},
	}
	recovered := productRun{
		Name: "prodex", ExitStatus: 0,
		Client:   exchange{Status: 200, Body: syntheticFixtureResponse},
		Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: syntheticFixtureRequest}},
	}
	scenario := scenarioResult{Name: "recover-after-503", Runs: []productRun{outage, recovered, outage, recovered}}
	if violations := scenarioInvariants(scenario); len(violations) != 0 {
		t.Fatalf("valid provider recovery rejected: %v", violations)
	}
	scenario.Runs[3] = outage
	if violations := scenarioInvariants(scenario); len(violations) == 0 {
		t.Fatal("persisted health state that blocks later healthy request was ignored")
	}
}

func TestMultipleCredentialsRotateOnlyAfterFailedPrimary(t *testing.T) {
	request := func(slot string) upstreamRequest {
		return upstreamRequest{
			Method: "POST", Path: "/v1/chat/completions", AuthOK: true,
			KeySlot: slot, Body: syntheticFixtureRequest,
		}
	}
	passed := productRun{
		ExitStatus: 0, Client: exchange{Status: 200, Body: syntheticFixtureResponse},
		Upstream: []upstreamRequest{request("primary"), request("secondary")},
		Retries:  1,
	}
	if failures := scenarioInvariants(scenarioResult{Name: "key-rotation-429", Runs: []productRun{passed, passed}}); len(failures) != 0 {
		t.Fatalf("correct independent rotation rejected: %v", failures)
	}
	mutated := passed
	mutated.Upstream = []upstreamRequest{request("primary"), request("primary")}
	if failures := scenarioInvariants(scenarioResult{Name: "key-rotation-429", Runs: []productRun{passed, mutated}}); len(failures) == 0 {
		t.Fatal("same credential replayed after 429 but oracle passed")
	}
}

func TestAllSyntheticProviderCredentialsAreRedacted(t *testing.T) {
	for _, key := range []string{apiKey, rotationPrimaryKey, rotationSecondaryKey} {
		if got := redact("credential=" + key); got != "credential=<synthetic-key>" {
			t.Fatalf("synthetic credential leaked: %q", got)
		}
	}
}

func TestRotationRestartRejectsWrongCredentialAfterPersistence(t *testing.T) {
	request := func(slot string) upstreamRequest {
		return upstreamRequest{Method: "POST", Path: "/v1/chat/completions", AuthOK: true,
			KeySlot: slot, Body: syntheticFixtureRequest}
	}
	first := productRun{ExitStatus: 0, Client: exchange{Status: 200, Body: syntheticFixtureResponse},
		Upstream: []upstreamRequest{request("primary"), request("secondary")}, Retries: 1}
	next := productRun{ExitStatus: 0, Client: exchange{Status: 200, Body: syntheticFixtureResponse},
		Upstream: []upstreamRequest{request("secondary")}, Retries: 0}
	scenario := scenarioResult{Name: "key-rotation-restart", Runs: []productRun{first, next, first, next}}
	if failures := scenarioInvariants(scenario); len(failures) != 0 {
		t.Fatalf("expected healthy second process rejected: %v", failures)
	}
	wrong := next
	wrong.Upstream = []upstreamRequest{request("primary")}
	scenario.Runs[3] = wrong
	if failures := scenarioInvariants(scenario); len(failures) == 0 {
		t.Fatal("persisted routing selected a failed primary but passed")
	}
}

func TestToolCallFixtureRequiresExactCallIdentityAndArguments(t *testing.T) {
	valid := `{"object":"response","model":"deepseek-v4-pro","output":[{"type":"function_call","call_id":"call-differential","name":"lookup","arguments":"{\"key\":\"alpha\"}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
	if !validFixtureToolCallResponse(valid) {
		t.Fatal("correct fixed function call rejected")
	}
	for _, body := range []string{
		`{"object":"response","model":"deepseek-v4-pro","output":[{"type":"function_call","call_id":"wrong","name":"lookup","arguments":"{}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
		`{"object":"response","model":"deepseek-v4-pro","output":[{"type":"function_call","call_id":"call-differential","name":"lookup","arguments":"{\"key\":\"beta\"}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
		`{"object":"response","model":"deepseek-v4-pro","output":[{"type":"message","call_id":"call-differential","name":"lookup","arguments":"{\"key\":\"alpha\"}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
		`{"object":"response","model":"deepseek-v4-pro","output":[{"type":"function_call","call_id":"call-differential","name":"lookup","arguments":"{\"key\":\"alpha\"}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":4}}`,
	} {
		if validFixtureToolCallResponse(body) {
			t.Fatalf("incorrect tool-call was accepted: %s", body)
		}
	}
}

func TestProviderAuthErrorOracleRejectsMatchingButWrongErrors(t *testing.T) {
	fixtures := []struct {
		code string
		ok   string
		bad  []string
	}{
		{"invalid_api_key",
			`{"error":{"code":"invalid_api_key","message":"synthetic credential rejected"}}`,
			[]string{
				`{"error":{"code":"rate_limit_exceeded","message":"synthetic credential rejected"}}`,
				`{"error":{"code":"invalid_api_key","message":"unknown"}}`,
				`{"status":"unauthorized"}`,
			}},
		{"access_denied",
			`{"error":{"code":"access_denied","message":"synthetic provider forbidden"}}`,
			[]string{
				`{"error":{"code":"access_denied","message":"credentials revoked"}}`,
				`{"error":{"code":"invalid_api_key","message":"synthetic provider forbidden"}}`,
				`{"error":{"message":"synthetic provider forbidden"}}`,
			}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.code, func(t *testing.T) {
			if !validFixtureError(fixture.ok, fixture.code) {
				t.Fatal("canonical synthetic provider error rejected")
			}
			for _, bad := range fixture.bad {
				if validFixtureError(bad, fixture.code) {
					t.Fatalf("mutated provider error accepted: %s", bad)
				}
			}
		})
	}
}

// The canonical DeepSeek translator commits before exposing provider errors
// as SSE. Retries after translation would hide the actual failure from Codex.
func TestProdex04370DeepSeekEmbeddedSSEErrorIsTerminal(t *testing.T) {
	wire := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"Please try again in 1s.\"},\"id\":\"resp_deepseek_01a11f94-b55e-73ce-ac35-d0f0438a99f3\"},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", time.Now().Unix())
	run := productRun{
		Name:       "prodex",
		ExitStatus: 0,
		Client:     exchange{Status: 200, Body: wire, Headers: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}},
		Upstream: []upstreamRequest{{
			Method: "POST", Path: "/v1/chat/completions", AuthOK: true,
			KeySlot: "primary", Body: syntheticFixtureRequest,
		}},
	}
	scenario := scenarioResult{Name: "deepseek-sse-terminal", Runs: []productRun{run, run}}
	if failures := scenarioInvariants(scenario); len(failures) != 0 {
		t.Fatalf("canonical terminal DeepSeek SSE rejected: %v", failures)
	}
	mutated := run
	mutated.Upstream = append(append([]upstreamRequest{}, run.Upstream...), upstreamRequest{
		Method: "POST", Path: "/v1/chat/completions", AuthOK: true,
		KeySlot: "secondary", Body: syntheticFixtureRequest,
	})
	scenario.Runs[1] = mutated
	if failures := scenarioInvariants(scenario); len(failures) == 0 {
		t.Fatal("provider error retried on secondary key without detection")
	}
	for _, corrupt := range []string{
		strings.Replace(wire, "rate_limit_exceeded", "wrong_code", 1),
		strings.Replace(wire, "Please try again in 1s.", "incorrect message", 1),
		strings.Replace(wire, "resp_deepseek_01a11f94-b55e-73ce-ac35-d0f0438a99f3", "invalid-id", 1),
		wire + "event: response.completed\r\ndata: {}\r\n\r\n",
	} {
		if validFixtureFailedSSE(corrupt) || equivalentFixtureFailedSSE(wire, corrupt) {
			t.Fatal("semantically incorrect terminal provider stream accepted")
		}
	}
}
