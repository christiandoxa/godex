package deepseek

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// These are the official Prodex 0.437.13 DeepSeek stream policy cases from
// crates/prodex-provider-core/src/translators/deepseek.rs. A Go JSON decoder
// alone tolerates unmatched escaped surrogates and much deeper nesting than
// the tagged Mojo policy/serde_json boundary permits.
func TestProdex043713DeepSeekStreamArgumentsMatchTaggedJSONBoundaries(t *testing.T) {
	tests := []struct {
		name, arguments string
		valid           bool
	}{
		{"lone high surrogate", `{"value":"\uD800"}`, false},
		{"lone low surrogate", `{"value":"\uDC00"}`, false},
		{"high then non-low", `{"value":"\uD800\u0041"}`, false},
		{"valid surrogate pair", `{"value":"\uD83D\uDE00"}`, true},
		{"double escaped surrogate", `{"value":"\\uD800"}`, true},
		{"float overflow", `{"value":1e309}`, false},
		{"float max-range", `{"value":1e308}`, true},
		{"scaled float finite", `{"value":0.1e309}`, true},
		{"scaled float overflow", `{"value":0.1e310}`, false},
		{"huge exponent of zero", `{"value":0e999999999999999999999999}`, true},
		{"huge negative exponent", `{"value":1e-999999999999999999999999}`, true},
		{"nested arrays 127", strings.Repeat("[", 127) + "0" + strings.Repeat("]", 127), true},
		{"nested arrays 128", strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128), false},
		{"nested arrays 129", strings.Repeat("[", 129) + "0" + strings.Repeat("]", 129), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var call deepSeekChatStreamToolCall
			call.name = "shell"
			call.arguments.WriteString(tt.arguments)
			err := deepSeekValidateStreamToolCall(5, &call)
			if got := err == nil; got != tt.valid {
				t.Fatalf("arguments %q accepted=%t want %t: %v", tt.arguments, got, tt.valid, err)
			}
		})
	}
}

// Exercise the actual SSE translation boundary: malformed escaped surrogate
// sequences and excessive nesting must end in response.failed without a
// response.completed event, rather than merely failing a helper in isolation.
func TestProdex043713DeepSeekSSERejectsInvalidToolArgumentsWithoutCompleting(t *testing.T) {
	for _, tc := range []struct {
		name, arguments string
		valid           bool
	}{
		{"high surrogate", `{"cmd":"\uD800"}`, false},
		{"deep array", strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128), false},
		{"valid surrogate pair", `{"cmd":"\uD83D\uDE00"}`, true},
		{"bounded array", strings.Repeat("[", 127) + "0" + strings.Repeat("]", 127), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := json.Marshal(map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": "call_713", "function": map[string]any{"name": "shell", "arguments": tc.arguments},
					}}},
					"finish_reason": "tool_calls",
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			stream := "data: " + string(chunk) + "\n\ndata: [DONE]\n\n"
			result := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(stream)))
			if tc.valid {
				if !strings.Contains(result, "event: response.completed") || strings.Contains(result, "event: response.failed") {
					t.Fatalf("valid DeepSeek SSE tool args did not complete: %s", result)
				}
			} else if !strings.Contains(result, "event: response.failed") ||
				!strings.Contains(result, `"code":"invalid_tool_call_arguments"`) ||
				strings.Contains(result, "event: response.completed") {
				t.Fatalf("invalid tool arguments escaped streaming validation: %s", result)
			}
		})
	}
}
