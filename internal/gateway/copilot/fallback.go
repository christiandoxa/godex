package copilot

import (
	"bytes"
	"encoding/json"
	"strings"
)

const (
	copilotFallbackLegacyCodex = "gpt-5.1-codex"
	copilotFallbackGPT4O       = "gpt-4o"
)

func copilotModelFallbackChain(body []byte) []string {
	object, ok := decodeJSONObject(body)
	if !ok {
		return []string{defaultRuntimeModel}
	}
	model, _ := object["model"].(string)
	trimmed := strings.TrimSpace(model)
	switch strings.ToLower(trimmed) {
	case "", "auto", "default", "codex":
		return []string{defaultRuntimeModel, copilotFallbackLegacyCodex, copilotFallbackGPT4O}
	case "gpt-5.5":
		return []string{"gpt-5.5", defaultRuntimeModel, copilotFallbackLegacyCodex, copilotFallbackGPT4O}
	case "gpt-5.4":
		return []string{"gpt-5.4", defaultRuntimeModel, copilotFallbackLegacyCodex, copilotFallbackGPT4O}
	case "gpt-5.3-codex":
		return []string{defaultRuntimeModel, copilotFallbackLegacyCodex, copilotFallbackGPT4O}
	case "sonnet":
		return []string{"claude-sonnet-4-6", defaultRuntimeModel, copilotFallbackLegacyCodex}
	case "gemini":
		return []string{"gemini-3.1-pro-preview", defaultRuntimeModel, copilotFallbackLegacyCodex}
	default:
		canonical := canonicalCopilotModel(trimmed)
		if canonical == "" {
			canonical = defaultRuntimeModel
		}
		return []string{canonical}
	}
}

func copilotRequestBodyWithModel(body []byte, model string) []byte {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return append([]byte(nil), body...)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return append([]byte(nil), body...)
	}
	object["model"] = strings.TrimSpace(model)
	stripEncryptedContent(object, false)
	rewritten, err := json.Marshal(object)
	if err != nil {
		return append([]byte(nil), body...)
	}
	return rewritten
}
