package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runRawKeyContinuationProbe(root string, mock *mockServer, prodexBin, godexBin, prodexCommit, godexCommit string) (scenarioResult, error) {
	result := scenarioResult{Name: "raw-key-continuation-probe"}
	for _, product := range []struct{ name, binary, commit string }{
		{"prodex-raw-key", prodexBin, prodexCommit}, {"godex-raw-key", godexBin, godexCommit},
	} {
		for _, phase := range rawKeyProbePhases {
			plan := mockPlan{RotateKeys: phase == "seed"}
			if phase == "fresh" || phase == "unknown" {
				plan.SSEFraming = "raw-key-fresh"
			}
			options := runOptions{rotate: phase != "resume-key", continuationProbe: phase}
			if phase != "seed" {
				options.priorBindings = rawKeyProbeSeedOwner(mock.baseURL() + "/v1")
			}
			if phase == "resume-endpoint" {
				options.endpointPath = "/different/v1"
			}
			run, err := runProduct(filepath.Join(root, "raw-key-continuation-probe"), mock, product.name,
				product.binary, product.commit, plan, options)
			if err != nil {
				return result, err
			}
			result.Runs = append(result.Runs, run)
		}
	}
	for phase := range rawKeyProbePhases {
		result.Comparison = append(result.Comparison, compare(result.Runs[phase], result.Runs[phase+len(rawKeyProbePhases)])...)
	}
	result.Comparison = uniqueStrings(result.Comparison)
	return result, nil
}

var rawKeyProbePhases = []string{"seed", "resume", "resume-key", "resume-endpoint", "resume-compact", "resume-models", "resume-models-single", "fresh", "unknown"}

func rawKeyProbeSeedOwner(endpoint string) map[string]fixtureContinuationOwner {
	return map[string]fixtureContinuationOwner{"chatcmpl-differential": {
		credential: fixtureBindingDigest("credential", rotationSecondaryKey),
		endpoint:   fixtureBindingDigest("endpoint", endpoint),
	}}
}

func rawKeyContinuationProbeInvariants(scenario scenarioResult) []string {
	if len(scenario.Runs) != 2*len(rawKeyProbePhases) {
		return []string{"raw-key-continuation-probe.run_count"}
	}
	var failures []string
	for index, run := range scenario.Runs {
		phase := rawKeyProbePhases[index%len(rawKeyProbePhases)]
		prefix := "raw-key-continuation-probe." + run.Name + "." + phase
		body := run.Client.Body
		validIdentity := true
		if phase == "fresh" || phase == "unknown" {
			body = strings.Replace(body, "chatcmpl-fresh", "chatcmpl-differential", 1)
			validIdentity = body != run.Client.Body
		}
		validResponse := validIdentity && run.ExitStatus == 0 && run.Client.Status == http.StatusOK && validFixtureResponse(body)
		if rawKeyLocalRouteFixture04384(phase) != "" {
			validResponse = validRawKeyLocalRoute04384(phase, run)
		} else if strings.HasPrefix(phase, "resume") {
			validResponse = rawKeyProbeRejected(run)
		}
		if !validResponse {
			failures = append(failures, prefix+".response")
		}
		var want []string
		if phase == "seed" {
			want = []string{"primary", "secondary"}
		} else if phase == "fresh" || phase == "unknown" {
			want = []string{"primary"}
		}
		if len(run.Upstream) != len(want) {
			failures = append(failures, prefix+".attempt_count")
		}
		if run.Retries != max(0, len(want)-1) {
			failures = append(failures, prefix+".retry_count")
		}
		for attempt, request := range run.Upstream {
			if attempt >= len(want) || request.KeySlot != want[attempt] || !request.AuthOK ||
				request.Method != http.MethodPost || request.Path != "/v1/chat/completions" || !validFixtureRequest(request.Body) {
				failures = append(failures, prefix+".exact_owner_or_request")
			}
		}
		if len(run.StateIntegrity) != 0 {
			failures = append(failures, prefix+".state_integrity")
		}
		if !run.ContinuationPersisted {
			failures = append(failures, prefix+".persistence_missing")
		}
	}
	return failures
}

func rawKeyProbeRejected(run productRun) bool {
	return run.ExitStatus == 2 && run.Client.Status == http.StatusServiceUnavailable &&
		run.Client.Body == `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}`
}

func rawKeyInProcessProbeInvariants(scenario scenarioResult) []string {
	if len(scenario.Runs) != 2 {
		return []string{"raw-key-in-process-probe.run_count"}
	}
	var failures []string
	for _, run := range scenario.Runs {
		prefix := "raw-key-in-process-probe." + run.Name
		if !rawKeyProbeRejected(run) || len(run.Upstream) != 2 || run.Retries != 1 || len(run.StateIntegrity) != 0 {
			failures = append(failures, prefix+".rejection_or_attempts_or_state")
		}
		for index, request := range run.Upstream {
			if index > 1 || request.KeySlot != []string{"primary", "secondary"}[index] || !request.AuthOK ||
				request.Method != http.MethodPost || request.Path != "/v1/chat/completions" || !validFixtureRequest(request.Body) {
				failures = append(failures, prefix+".exact_owner_or_request")
			}
		}
	}
	return failures
}

func waitRawKeyProbePersistence(root string, prodex bool, responseID string) {
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for !rawKeyProbeBindingExists(root, prodex, responseID) {
		select {
		case <-deadline.C:
			return
		case <-tick.C:
		}
	}
}

func rawKeyProbeBindingExists(root string, prodex bool, responseID string) bool {
	name := "routing.json"
	if prodex {
		name = "runtime-continuations.json"
	}
	file, err := os.Open(filepath.Join(root, name))
	if err != nil {
		return false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFixtureStateFileBytes+1))
	if err != nil || len(data) > maxFixtureStateFileBytes {
		return false
	}
	var state map[string]any
	if json.Unmarshal(data, &state) != nil {
		return false
	}
	if prodex {
		value, _ := state["value"].(map[string]any)
		bindings, _ := value["response_profile_bindings"].(map[string]any)
		return bindings[responseID] != nil
	}
	bindings, _ := state["bindings"].([]any)
	for _, raw := range bindings {
		binding, _ := raw.(map[string]any)
		if kind, _ := binding["kind"].(string); kind == "previous" && binding["key"] == fixturePreviousDigest(responseID) {
			return true
		}
	}
	return false
}
