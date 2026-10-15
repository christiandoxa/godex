package gemini

import "testing"

// Prodex 0.437.13 mojo/prodex_core/gemini_response.mojo unwraps the
// nested response even when no traceId is present. Only a string traceId
// overrides the nested canonical responseId.
func TestProdex043713GeminiNativeEnvelopeWithoutTracePreservesNestedResponse(t *testing.T) {
	nested := map[string]any{"responseId": "nested-response", "modelVersion": "gemini-3.1-pro-preview",
		"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "synthetic-ok"}}}}},
	}
	for _, tc := range []struct {
		name     string
		trace    any
		hasTrace bool
		wantID   string
	}{
		{name: "missing trace", wantID: "nested-response"},
		{name: "nonstring trace", trace: 7, hasTrace: true, wantID: "nested-response"},
		{name: "valid trace", trace: "trusted-trace", hasTrace: true, wantID: "trusted-trace"},
		{name: "empty trace", trace: "", hasTrace: true, wantID: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := map[string]any{"response": nested, "decoy": "outer"}
			if tc.hasTrace {
				root["traceId"] = tc.trace
			}
			actual := normalizedGeminiResponse(root)
			if actual["responseId"] != tc.wantID || actual["modelVersion"] != "gemini-3.1-pro-preview" || actual["decoy"] != nil {
				t.Fatalf("tagged nested Gemini envelope not normalized: %#v", actual)
			}
			if nested["responseId"] != "nested-response" {
				t.Fatal("normalizer mutated the original nested provider document")
			}
		})
	}
}
