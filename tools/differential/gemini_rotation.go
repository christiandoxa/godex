package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
)

// The official Prodex 0.437.11 distinguishes a bare 429 quota text from
// structured rate-limit JSON. Only a retryable structured error may rotate
// a fresh request to the next API-key credential before output commitment.
func geminiQuotaScenario043711(scenario string) bool {
	return scenario == "quota-plain-429" || scenario == "quota-structured-429"
}

// Never expose even synthetic credential bytes in an oracle's report.
func geminiCredentialSlot043711(request *http.Request) string {
	bearer := request.Header.Get("Authorization")
	key := request.Header.Get("X-Goog-Api-Key")
	if key == "" {
		key = request.URL.Query().Get("key")
	}
	for _, option := range []struct{ value, slot string }{
		{apiKey, "single"},
		{rotationPrimaryKey, "primary"},
		{rotationSecondaryKey, "secondary"},
	} {
		if bearer == "Bearer "+option.value || key == option.value {
			return option.slot
		}
	}
	return ""
}

func geminiQuotaInvariant043711(scenario string, run geminiCandidateResult) error {
	wantAttempts, wantStatus := 1, http.StatusTooManyRequests
	if scenario == "quota-structured-429" {
		wantAttempts, wantStatus = 6, http.StatusOK
	}
	wantExit := 2
	if scenario == "quota-structured-429" {
		wantExit = 0
	}
	if run.ExitCode != wantExit {
		return fmt.Errorf("Gemini %s child exit=%d, want %d", scenario, run.ExitCode, wantExit)
	}
	if run.Client.Status != wantStatus {
		return fmt.Errorf("Gemini %s client status=%d, want %d", scenario, run.Client.Status, wantStatus)
	}
	if len(run.Upstream) != wantAttempts {
		return fmt.Errorf("Gemini %s sent %d upstream attempts, want %d", scenario, len(run.Upstream), wantAttempts)
	}
	expectedModels := []string{"gemini-3.1-pro-preview"}
	if scenario == "quota-structured-429" {
		// Pinned Prodex 0.437.11 fallback tries five models on the
		// first credential, then restarts the chain on the second.
		expectedModels = []string{
			"gemini-3.1-pro-preview", "gemini-3.8-flash",
			"gemini-3.7-flash", "gemini-3.6-flash",
			"gemini-2.5-pro", "gemini-3.1-pro-preview",
		}
	}
	for index, request := range run.Upstream {
		slot := "primary"
		if index == wantAttempts-1 && scenario == "quota-structured-429" {
			slot = "secondary"
		}
		if !request.Auth || request.Slot != slot ||
			request.Method != http.MethodPost ||
			request.Path != "/v1beta/openai/chat/completions" ||
			request.Body["model"] != expectedModels[index] ||
			request.Body["stream"] != false {
			return fmt.Errorf("Gemini %s attempt %d wrong method/path/credential/model: %+v",
				scenario, index, request)
		}
		messages, _ := request.Body["messages"].([]any)
		if len(messages) != 1 {
			return fmt.Errorf("Gemini %s attempt %d lost its user request: %+v", scenario, index, messages)
		}
		user, _ := messages[0].(map[string]any)
		if user["role"] != "user" || user["content"] != "same request" {
			return fmt.Errorf("Gemini %s attempt %d changed user message: %+v", scenario, index, user)
		}
	}
	if scenario == "quota-plain-429" {
		if run.Client.Body != "Quota exhausted for this account\n" {
			return fmt.Errorf("Gemini unstructured 429 not preserved verbatim: %q", run.Client.Body)
		}
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Client.Body), &payload); err != nil {
		return fmt.Errorf("Gemini structured quota rotation final response invalid JSON: %w", err)
	}
	if err := geminiBufferedIdentityInvariant043711(payload); err != nil {
		return err
	}
	output, _ := payload["output"].([]any)
	usage, _ := payload["usage"].(map[string]any)
	metadata, _ := payload["metadata"].(map[string]any)
	gemini, _ := metadata["gemini"].(map[string]any)
	if payload["object"] != "response" ||
		payload["model"] != "gemini-3.1-pro-preview" ||
		len(output) != 1 ||
		usage["input_tokens"] != float64(2) ||
		usage["output_tokens"] != float64(1) ||
		usage["total_tokens"] != float64(3) ||
		gemini["finishReason"] != "STOP" {
		return fmt.Errorf("Gemini quota rotation lost final native response or usage: %+v", payload)
	}
	message, _ := output[0].(map[string]any)
	parts, _ := message["content"].([]any)
	if len(parts) != 1 {
		return fmt.Errorf("Gemini quota rotation lost final provider content: %+v", payload)
	}
	text, _ := parts[0].(map[string]any)
	if text["text"] != "synthetic-ok" {
		return fmt.Errorf("Gemini quota rotation changed final provider content: %+v", payload)
	}
	return nil
}

func geminiQuotaCompare043711(
	scenario string, prodex, godex geminiCandidateResult, differences []string,
) []string {
	if len(prodex.Upstream) != len(godex.Upstream) {
		differences = append(differences, "Gemini quota upstream attempt count differs")
	}
	for index := 0; index < min(len(prodex.Upstream), len(godex.Upstream)); index++ {
		if !reflect.DeepEqual(prodex.Upstream[index], godex.Upstream[index]) {
			differences = append(differences, fmt.Sprintf("Gemini quota upstream request %d differs", index))
		}
	}
	if scenario == "quota-plain-429" {
		if prodex.Client.Body != godex.Client.Body {
			differences = append(differences, "Gemini unstructured 429 client body differs")
		}
	} else {
		left, leftErr := geminiResponseProjection(prodex.Client.Body)
		right, rightErr := geminiResponseProjection(godex.Client.Body)
		if leftErr != nil || rightErr != nil || !geminiSemanticEqual043711(left, right) {
			differences = append(differences, "Gemini structured quota rotation final response differs")
		}
	}
	if len(differences) == 0 {
		return nil
	}
	return differences
}
