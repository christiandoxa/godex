package gemini

import "testing"

// Tagged 0.438.4 Gemini bridge's policy compares reasoning labels after
// folding A-Z ASCII bytes only, never Unicode U+0130 DOTTED CAPITAL I.
// Invalid Unicode lookalikes cannot enable, disable, or intensify thinking.
func TestProdex04384GeminiReasoningEffortUsesASCIIOnlyCasefold(t *testing.T) {
	for _, tc := range []struct {
		name, effort, want string
		accepted           bool
	}{
		{"canonical-high", "high", "high", true},
		{"uppercase-high", "HIGH", "high", true},
		{"mixedcase-low", "LoW", "low", true},
		{"canonical-minimal", "MINIMAL", "minimal", true},
		{"dotted-I-high", "HİGH", "", false},
		{"dotted-I-minimal", "MİNİMAL", "", false},
		{"dotted-I-medium", "MEDİUM", "", false},
		{"dotted-I-xhigh", "XHİGH", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, input := range []map[string]any{
				{"reasoning_effort": tc.effort},
				{"reasoning": map[string]any{"effort": tc.effort}},
			} {
				actual, err := geminiReasoningEffort(input)
				if (err == nil) != tc.accepted || (err == nil && actual != tc.want) {
					t.Fatalf("Gemini effort=%q accepted=%t normalized=%q want=%q valid=%t", tc.effort, err == nil, actual, tc.want, tc.accepted)
				}
			}
		})
	}
}

func TestProdex04384GeminiReasoningUnicodeCannotChangeThinkingConfig(t *testing.T) {
	for _, tc := range []struct {
		name, effort string
		valid        bool
	}{
		{"valid-off", "minimal", true},
		{"valid-high", "HIGH", true},
		{"invalid-off", "MİNİMAL", false},
		{"invalid-high", "HİGH", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"reasoning": map[string]any{"effort": tc.effort}}
			effort, err := geminiReasoningEffort(input)
			if (err == nil) != tc.valid {
				t.Fatalf("untrusted effort %q accepted=%t expected=%t", tc.effort, err == nil, tc.valid)
			}
			if !tc.valid {
				return
			}
			config := geminiThinkingConfig(input, "gemini-3.1-pro-preview", effort)
			disabled := tc.effort == "minimal"
			if config["includeThoughts"] != !disabled {
				t.Fatalf("thinking config %+v for %s", config, tc.effort)
			}
		})
	}
}
