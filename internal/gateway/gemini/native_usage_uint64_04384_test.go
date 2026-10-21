package gemini

import (
	"encoding/json"
	"testing"
)

// The 0.438.4 gemini_response.mojo gemini_raw_u64 checks raw JSON digits
// against the full u64 bound. It does NOT accept 18.0, 1e2, negative or
// textual numeric values. Serde serializes floats with a decimal point.
func TestProdex04384GeminiNativeUsagePreservesUnsigned64AndRejectsFloat(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		input, output, total             any
		wantInput, wantOutput, wantTotal uint64
	}{
		{"ordinary", json.Number("2"), json.Number("3"), nil, 2, 3, 5},
		{"full-u64", json.Number("18446744073709551615"), json.Number("1"), nil, ^uint64(0), 1, ^uint64(0)},
		{"above-signed-max", json.Number("9223372036854775808"), json.Number("2"), nil, 1 << 63, 2, (uint64(1) << 63) + 2},
		{"invalid-over-u64", json.Number("18446744073709551616"), json.Number("2"), nil, 0, 2, 2},
		{"invalid-decimal", json.Number("18.0"), json.Number("2"), nil, 0, 2, 2},
		{"invalid-exponent", json.Number("1e2"), json.Number("2"), nil, 0, 2, 2},
		{"invalid-negative", json.Number("-1"), json.Number("2"), nil, 0, 2, 2},
		{"explicit-zero-total", json.Number("2"), json.Number("3"), json.Number("0"), 2, 3, 0},
		{"invalid-total-float", json.Number("2"), json.Number("3"), json.Number("18.0"), 2, 3, 5},
		{"float64-integer-looking", float64(18), json.Number("2"), nil, 0, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"promptTokenCount": tc.input, "candidatesTokenCount": tc.output}
			if tc.total != nil {
				input["totalTokenCount"] = tc.total
			}
			result := geminiNativeUsage(input)
			if result["input_tokens"] != tc.wantInput || result["output_tokens"] != tc.wantOutput || result["total_tokens"] != tc.wantTotal {
				t.Fatalf("Gemini raw numeric usage %s got=%#v expected input=%d output=%d total=%d", tc.name, result, tc.wantInput, tc.wantOutput, tc.wantTotal)
			}
		})
	}
}

func TestProdex04384GeminiNativeUsageFullUnsignedDetails(t *testing.T) {
	input := map[string]any{
		"promptTokenCount":        json.Number("18446744073709551615"),
		"candidatesTokenCount":    json.Number("18446744073709551615"),
		"cachedContentTokenCount": json.Number("9223372036854775808"),
		"thoughtsTokenCount":      json.Number("18446744073709551615"),
		"toolUsePromptTokenCount": json.Number("18.0"),
	}
	usage := geminiNativeUsage(input)
	details, _ := usage["input_tokens_details"].(map[string]any)
	thoughts, _ := usage["output_tokens_details"].(map[string]any)
	if usage["total_tokens"] != ^uint64(0) || details["cached_tokens"] != uint64(1)<<63 || details["tool_tokens"] != uint64(0) || thoughts["reasoning_tokens"] != ^uint64(0) {
		t.Fatalf("Gemini u64 bounded usage details mismatch: %#v", usage)
	}
}
