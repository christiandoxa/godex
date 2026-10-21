package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
)

type geminiModelCase04384 struct {
	query, expectedID string
	status            int
}

var geminiModelCases04384 = map[string]geminiModelCase04384{
	"models-canonical":          {"gemini-3.1-pro-preview", "gemini-3.1-pro-preview", http.StatusOK},
	"models-uppercase":          {"GEMINI-3.1-PRO-PREVIEW", "gemini-3.1-pro-preview", http.StatusOK},
	"models-alias":              {"default", "auto", http.StatusOK},
	"models-padded-id":          {" gemini-3.1-pro-preview ", "", http.StatusBadRequest},
	"models-padded-alias":       {" default ", "", http.StatusBadRequest},
	"models-unicode-confusable": {"gemini-3.1-pro-prevİew", "", http.StatusBadRequest},
	"models-unknown":            {"no-such-synthetic-model", "", http.StatusNotFound},
}

var geminiModelCaseOrder04384 = []string{
	"models-canonical", "models-uppercase", "models-alias", "models-padded-id",
	"models-padded-alias", "models-unicode-confusable", "models-unknown",
}

func geminiModelScenario04384(scenario string) bool {
	_, found := geminiModelCases04384[scenario]
	return found
}

func geminiModelCase04384For(scenario string) (geminiModelCase04384, bool) {
	value, found := geminiModelCases04384[scenario]
	return value, found
}

// Model endpoints terminate within the provider bridge, before any API-key
// network call. Compare both separately authenticated pinned executables
// using the same synthetic Codex shim and empty upstream loopback capture.
func geminiModelFixtureInvariant04384(scenario string, result geminiCandidateResult) error {
	fixture, valid := geminiModelCase04384For(scenario)
	if !valid {
		return errors.New("unknown Gemini model fixture")
	}
	if len(result.Upstream) != 0 {
		return fmt.Errorf("Gemini model catalog leaked %d upstream requests", len(result.Upstream))
	}
	if result.Client.Status != fixture.status {
		return fmt.Errorf("Gemini model %q returned HTTP %d want %d", fixture.query, result.Client.Status, fixture.status)
	}
	wantExit := 0
	if fixture.status != http.StatusOK {
		wantExit = 2
	}
	if result.ExitCode != wantExit {
		return fmt.Errorf("Gemini model %q exit=%d want=%d", fixture.query, result.ExitCode, wantExit)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result.Client.Body), &decoded); err != nil {
		return err
	}
	if fixture.status == http.StatusOK {
		if decoded["object"] != "model" || decoded["id"] != fixture.expectedID {
			return fmt.Errorf("Gemini model %q wrong resolved identity: %#v", fixture.query, decoded)
		}
	} else {
		detail, _ := decoded["error"].(map[string]any)
		if fixture.status == http.StatusBadRequest {
			if detail["code"] != "invalid_request_target" || detail["message"] != "request target is invalid" {
				return fmt.Errorf("Gemini unsafe model target %q wrong 400: %#v", fixture.query, decoded)
			}
		} else if detail["code"] != "model_not_found" {
			return fmt.Errorf("Gemini model %q wrong 404: %#v", fixture.query, decoded)
		}
	}
	return nil
}

func geminiModelCompare04384(scenario string, prodex, godex geminiCandidateResult, differences []string) []string {
	var left, right map[string]any
	if json.Unmarshal([]byte(prodex.Client.Body), &left) != nil || json.Unmarshal([]byte(godex.Client.Body), &right) != nil || !reflect.DeepEqual(left, right) {
		differences = append(differences, "Gemini models endpoint response differs")
	}
	return differences
}

func runGeminiModelDifferential04384(prodexBin, godexBin, prodexCommit, godexCommit string) error {
	return runGeminiDifferentialCases04384(prodexBin, godexBin, prodexCommit, godexCommit, geminiModelCaseOrder04384)
}
