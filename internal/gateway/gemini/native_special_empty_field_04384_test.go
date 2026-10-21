package gemini

import "testing"

// Source 0.438.4 media_put_special_text uses pj_string_field presence.
// Present empty strings are explicit values, not missing fields: the
// default language/outcome appears only when the field is absent/nonstring.
func TestProdex04384GeminiSpecialMediaExplicitEmptyLabelsRemainEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		part map[string]any
		want string
	}{
		{"explicit-empty-code-language", map[string]any{"executableCode": map[string]any{"language": "", "code": "print(1)"}}, "Gemini executable code ():\n```\nprint(1)\n```"},
		{"missing-code-language-default", map[string]any{"executableCode": map[string]any{"code": "print(1)"}}, "Gemini executable code (text):\n```text\nprint(1)\n```"},
		{"invalid-code-language-default", map[string]any{"executableCode": map[string]any{"language": 42, "code": "print(1)"}}, "Gemini executable code (text):\n```text\nprint(1)\n```"},
		{"explicit-empty-execution-outcome", map[string]any{"codeExecutionResult": map[string]any{"outcome": "", "output": "ok"}}, "Gemini code execution result ():\n```text\nok\n```"},
		{"missing-execution-outcome-default", map[string]any{"codeExecutionResult": map[string]any{"output": "ok"}}, "Gemini code execution result (OUTCOME_UNSPECIFIED):\n```text\nok\n```"},
		{"invalid-execution-outcome-default", map[string]any{"codeExecutionResult": map[string]any{"outcome": false, "output": "ok"}}, "Gemini code execution result (OUTCOME_UNSPECIFIED):\n```text\nok\n```"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := geminiSpecialPartText(tc.part)
			if got != tc.want {
				t.Fatalf("Gemini special explicit label=%q expected=%q", got, tc.want)
			}
		})
	}
}
