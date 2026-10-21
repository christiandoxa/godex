package gemini

import "testing"

// The 0.438.4 gemini_bridge_request_string_contains kernel lowercases A-Z
// ASCII bytes when checking the model family. Uppercase model IDs must
// use thinkingLevel for Gemini 3 / Gemma 3 / Gemma 4, not a legacy
// thinkingBudget. Unicode lookalikes remain separate identities.
func TestProdex04384GeminiThinkingModelFamilyUsesASCIIOnlyCasefold(t *testing.T) {
	for _, tc := range []struct {
		name, model, effort string
		wantLevel           string
		wantBudget          int64
	}{
		{"gemini3-canonical-low", "gemini-3.1-pro-preview", "low", "LOW", 0},
		{"gemini3-uppercase-low", "GEMINI-3.1-PRO-PREVIEW", "low", "LOW", 0},
		{"gemini3-mixedcase-medium", "Gemini-3.1-Pro", "medium", "MEDIUM", 0},
		{"gemma3-uppercase", "GEMMA-3-27B-IT", "high", "HIGH", 0},
		{"gemma4-mixedcase", "Gemma-4-31B-IT", "medium", "MEDIUM", 0},
		{"non3-legacy", "gemini-2.5-pro", "low", "", 1024},
		{"unicode-dotted-I", "GEMİNİ-3.1-PRO", "low", "", 1024},
		{"model-substring-uppercase", "custom-GEMINI-3-model", "medium", "MEDIUM", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := geminiThinkingConfig(map[string]any{}, tc.model, tc.effort)
			if config["includeThoughts"] != true {
				t.Fatalf("model %q thoughts unexpectedly disabled: %#v", tc.model, config)
			}
			if tc.wantLevel != "" {
				if config["thinkingLevel"] != tc.wantLevel || config["thinkingBudget"] != nil {
					t.Fatalf("model %q got=%#v want level=%q", tc.model, config, tc.wantLevel)
				}
			} else if config["thinkingBudget"] != tc.wantBudget || config["thinkingLevel"] != nil {
				t.Fatalf("model %q got=%#v want budget=%d", tc.model, config, tc.wantBudget)
			}
		})
	}
}
