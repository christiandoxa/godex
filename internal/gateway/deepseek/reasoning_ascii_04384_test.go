package deepseek

import (
	"testing"
)

// The tagged 0.438.4 deepseek_trimmed_effort_equals trims Unicode space but
// folds ASCII A-Z ONLY. Go strings.ToLower silently converts U+0130 DOTTED
// CAPITAL I into ASCII i, potentially changing the model thinking mode.
func TestProdex04384DeepSeekReasoningEffortASCIIOnly(t *testing.T) {
	for _, tc := range []struct {
		name, effort string
		wantEffort   string
		valid        bool
	}{
		{"canonical-high", "high", "high", true},
		{"uppercase-high", "HIGH", "high", true},
		{"padded-medium", " MEDIUM ", "medium", true},
		{"canonical-minimal", "MINIMAL", "minimal", true},
		{"dotted-I-high", "HİGH", "", false},
		{"dotted-I-medium", "MEDİUM", "", false},
		{"dotted-I-minimal", "MİNİMAL", "", false},
		{"dotted-I-xhigh", "XHİGH", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, input := range []map[string]any{
				{"reasoning_effort": tc.effort},
				{"reasoning": map[string]any{"effort": tc.effort}},
			} {
				normalized, err := reasoningEffort(input)
				if err != nil {
					t.Fatal(err)
				}
				enabled, fields, err := deepSeekReasoning(input)
				if (err == nil) != tc.valid {
					t.Fatalf("effort=%q accepted=%t want=%t normalized=%q enabled=%t fields=%#v", tc.effort, err == nil, tc.valid, normalized, enabled, fields)
				}
				if tc.valid && normalized != tc.wantEffort {
					t.Fatalf("ASCII effort %q normalized=%q want=%q", tc.effort, normalized, tc.wantEffort)
				}
				if !tc.valid && (enabled || fields != nil) {
					t.Fatalf("invalid Unicode effort %q changed thinking=%t fields=%#v", tc.effort, enabled, fields)
				}
			}
		})
	}
}
