package gemini

import (
	"testing"
)

// Tagged 0.437.11 Mojo Gemini bridge hardens request.contents before the
// root-level fallback, and a nested functionCall signature is not a
// signature on the surrounding model part.
func TestProdex043711GeminiHardeningPrefersNestedRequestContents(t *testing.T) {
	root := map[string]any{"role": "model", "parts": []any{
		map[string]any{"functionCall": map[string]any{"name": "root"}},
	}}
	nested := map[string]any{"role": "model", "parts": []any{
		map[string]any{"functionCall": map[string]any{"name": "nested"}},
	}}
	body := map[string]any{
		"request":  map[string]any{"contents": []any{nested}},
		"contents": []any{root},
	}
	if got := hardenGeminiToolCallThoughtSignatures(body, "gemini-3.1-pro-preview"); got != 1 {
		t.Fatalf("nested Gemini3 request should inject exactly once, got %d", got)
	}
	nestedPart := nested["parts"].([]any)[0].(map[string]any)
	rootPart := root["parts"].([]any)[0].(map[string]any)
	if nestedPart["thoughtSignature"] != "skip_thought_signature_validator" {
		t.Fatalf("tagged nested request was not hardened: %+v", nestedPart)
	}
	if _, found := rootPart["thoughtSignature"]; found {
		t.Fatalf("root contents incorrectly hardened despite a valid request.contents: %+v", rootPart)
	}
}

func TestProdex043711GeminiHardeningUsesPartSignatureNotNestedCall(t *testing.T) {
	cases := []struct {
		name          string
		part          map[string]any
		wantInjection int
	}{
		{"nested call signature is insufficient", map[string]any{
			"functionCall": map[string]any{"name": "signed", "thoughtSignature": "nested"},
		}, 1},
		{"part signature stays intact", map[string]any{
			"functionCall": map[string]any{"name": "signed"}, "thoughtSignature": "signed",
		}, 0},
		{"part blank signature is not valid", map[string]any{
			"functionCall": map[string]any{"name": "signed"}, "thought_signature": "\u3000",
		}, 1},
		{"part null signature is not valid", map[string]any{
			"functionCall": map[string]any{"name": "signed"}, "thoughtSignature": nil,
		}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"contents": []any{map[string]any{
				"role": "model", "parts": []any{tc.part},
			}}}
			if got := hardenGeminiToolCallThoughtSignatures(body, "gemini-3"); got != tc.wantInjection {
				t.Fatalf("tagged Gemini signature presence: got injections=%d, want=%d part=%+v",
					got, tc.wantInjection, tc.part)
			}
			if tc.wantInjection == 1 && tc.part["thoughtSignature"] != "skip_thought_signature_validator" {
				t.Fatalf("synthetic signature missing at model part: %+v", tc.part)
			}
			if tc.wantInjection == 0 && tc.part["thoughtSignature"] != "signed" {
				t.Fatalf("existing model part signature rewritten: %+v", tc.part)
			}
		})
	}
}

func TestProdex043711GeminiHardeningLeavesNonGemini3NestedUntouched(t *testing.T) {
	part := map[string]any{"functionCall": map[string]any{"name": "noop"}}
	body := map[string]any{"request": map[string]any{"contents": []any{
		map[string]any{"role": "model", "parts": []any{part}},
	}}}
	if got := hardenGeminiToolCallThoughtSignatures(body, "gemini-2.5-pro"); got != 0 {
		t.Fatalf("Gemini2.5 changed thought signatures: %d", got)
	}
	if _, exists := part["thoughtSignature"]; exists {
		t.Fatalf("non-Gemini3 part was rewritten: %+v", part)
	}
}
