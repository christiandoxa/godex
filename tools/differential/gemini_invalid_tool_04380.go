package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

func geminiInvalidToolScenario04380(scenario string) bool {
	return scenario == "invalid-wrapped-tool-parameters" || scenario == "invalid-flat-tool-parameters"
}

// Only generated response IDs/timestamps can be ignored; an invalid function
// tool must be rejected at the *local* Gemini bridge with a stable status,
// code, indexed field path and error detail. The provider sees zero requests.
func geminiInvalidToolWireEqual04380(left, right string) bool {
	var first, second map[string]any
	if json.Unmarshal([]byte(left), &first) != nil || json.Unmarshal([]byte(right), &second) != nil {
		return false
	}
	return reflect.DeepEqual(first, second)
}

func geminiInvalidToolInvariant04380(scenario string, result geminiCandidateResult) error {
	if result.Client.Status != http.StatusBadRequest || result.ExitCode != 2 {
		return fmt.Errorf("Gemini invalid tool client status=%d child exit=%d, want 400/2", result.Client.Status, result.ExitCode)
	}
	if len(result.Upstream) != 0 {
		return fmt.Errorf("invalid Gemini tool leaked into %d provider requests", len(result.Upstream))
	}
	expectedField := "tools[0].function.parameters"
	expectedDetail := "must be an object"
	if scenario == "invalid-flat-tool-parameters" {
		expectedField = "tools[0].parameters"
		expectedDetail = "is required"
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Client.Body), &body); err != nil {
		return fmt.Errorf("invalid-tool response not JSON: %w", err)
	}
	errorObj, ok := body["error"].(map[string]any)
	if !ok {
		return errors.New("Gemini invalid-tool response lacked structured error")
	}
	message, _ := errorObj["message"].(string)
	if !strings.Contains(message, "invalid_tool_declaration") || !strings.Contains(message, expectedField) ||
		!strings.Contains(message, expectedDetail) {
		return fmt.Errorf("Gemini invalid-tool indexed diagnostic missing: %q", message)
	}
	return nil
}
