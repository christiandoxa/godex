package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawKeyContinuationProbeRejectsWrongRestartOwner(t *testing.T) {
	result := scenarioResult{Name: "raw-key-continuation-probe"}
	for product := range 2 {
		for _, phase := range rawKeyProbePhases {
			run := productRun{Name: []string{"prodex", "godex"}[product], ContinuationPersisted: true, Client: exchange{Status: http.StatusOK, Body: syntheticFixtureResponse}}
			var slots []string
			if phase == "seed" {
				slots = []string{"primary", "secondary"}
			} else if phase == "fresh" || phase == "unknown" {
				slots = []string{"primary"}
				run.Client.Body = strings.Replace(run.Client.Body, `"object":`, `"id":"chatcmpl-fresh","object":`, 1)
			} else {
				run.ExitStatus = 2
				run.Client.Status = http.StatusServiceUnavailable
				run.Client.Body = `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}`
			}
			run.Retries = max(0, len(slots)-1)
			for _, slot := range slots {
				run.Upstream = append(run.Upstream, upstreamRequest{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: slot, Body: syntheticFixtureRequest})
			}
			result.Runs = append(result.Runs, run)
		}
	}
	if failures := rawKeyContinuationProbeInvariants(result); len(failures) != 0 {
		t.Fatalf("valid owner trace rejected: %v", failures)
	}
	result.Runs[len(rawKeyProbePhases)+1].Upstream = []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "primary", Body: syntheticFixtureRequest}}
	if failures := rawKeyContinuationProbeInvariants(result); len(failures) == 0 {
		t.Fatal("unbound restart owner escaped probe")
	}
}

func TestRawKeyInProcessOracleRejectsIndependentMutations(t *testing.T) {
	run := productRun{ExitStatus: 2, Retries: 1, Client: exchange{Status: 503,
		Body: `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}`}}
	for _, slot := range []string{"primary", "secondary"} {
		run.Upstream = append(run.Upstream, upstreamRequest{Method: "POST", Path: "/v1/chat/completions",
			AuthOK: true, KeySlot: slot, Body: syntheticFixtureRequest})
	}
	valid := scenarioResult{Runs: []productRun{run, run}}
	if failures := rawKeyInProcessProbeInvariants(valid); len(failures) != 0 {
		t.Fatalf("valid in-process rejection rejected: %v", failures)
	}
	for _, mutate := range []func(*productRun){
		func(r *productRun) { r.ExitStatus = 0 },
		func(r *productRun) { r.Client.Status = 409 },
		func(r *productRun) { r.Client.Body = `{}` },
		func(r *productRun) { r.Retries = 0 },
		func(r *productRun) { r.StateIntegrity = []string{"secret_leak"} },
		func(r *productRun) { r.Upstream = r.Upstream[:1] },
		func(r *productRun) { r.Upstream = append(r.Upstream, r.Upstream[1]) },
	} {
		changed := run
		mutate(&changed)
		if failures := rawKeyInProcessProbeInvariants(scenarioResult{Runs: []productRun{changed, changed}}); len(failures) == 0 {
			t.Fatal("matching mutated failures escaped the independent oracle")
		}
	}
}

func TestRawKeyProbeWaitObservesOnlyWrittenBinding(t *testing.T) {
	for _, prodex := range []bool{false, true} {
		root := t.TempDir()
		if rawKeyProbeBindingExists(root, prodex, "chatcmpl-differential") {
			t.Fatal("absent state accepted")
		}
		name, body := "routing.json", `{"bindings":[{"kind":"previous","key":"`+fixturePreviousDigest("chatcmpl-differential")+`"}]}`
		if prodex {
			name, body = "runtime-continuations.json", `{"generation":1,"value":{"response_profile_bindings":{"chatcmpl-differential":{}}}}`
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if !rawKeyProbeBindingExists(root, prodex, "chatcmpl-differential") {
			t.Fatal("written fixture binding not observed")
		}
		if rawKeyProbeBindingExists(root, prodex, "chatcmpl-unknown") {
			t.Fatal("another response's binding satisfied persistence evidence")
		}
	}
}
