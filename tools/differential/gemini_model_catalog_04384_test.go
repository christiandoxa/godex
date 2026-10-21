package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProdex04384GeminiModelExecutableFixtureRejectsIdentityMutations(t *testing.T) {
	for _, name := range geminiModelCaseOrder04384 {
		t.Run(name, func(t *testing.T) {
			item, exists := geminiModelCase04384For(name)
			if !exists || item.query == "" {
				t.Fatal("missing Gemini model case")
			}
			body := map[string]any{}
			if item.status == http.StatusOK {
				body = map[string]any{"object": "model", "id": item.expectedID}
			} else if item.status == http.StatusBadRequest {
				body = map[string]any{"error": map[string]any{"code": "invalid_request_target", "message": "request target is invalid"}}
			} else {
				body = map[string]any{"error": map[string]any{"code": "model_not_found"}}
			}
			bytes, _ := json.Marshal(body)
			exit := 0
			if item.status != http.StatusOK {
				exit = 2
			}
			result := geminiCandidateResult{Client: exchange{Status: item.status, Body: string(bytes)}, ExitCode: exit}
			if err := geminiModelFixtureInvariant04384(name, result); err != nil {
				t.Fatalf("canonical Gemini model fixture rejected: %v", err)
			}
			if item.status == http.StatusOK {
				body["id"] = "wrong-model"
			} else {
				body["error"].(map[string]any)["code"] = "unknown_code"
			}
			bytes, _ = json.Marshal(body)
			result.Client.Body = string(bytes)
			if err := geminiModelFixtureInvariant04384(name, result); err == nil {
				t.Fatal("Gemini model identity mutation evaded fixture oracle")
			}
			result.Client.Body = "{}"
			result.Upstream = []geminiCapturedRequest{{Method: http.MethodGet, Path: "/should-not-contact-provider"}}
			if err := geminiModelFixtureInvariant04384(name, result); err == nil {
				t.Fatal("Gemini model route escaped to provider unnoticed")
			}
		})
	}
}
