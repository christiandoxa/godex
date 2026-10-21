package gemini

import "testing"

// Tagged 0.438.4 Rust response_tool_calls.rs presents only camelCase
// thoughtSignature from the part then the functionCall object. Compiled
// gemini_signature_choice returns after the FIRST present field, even when
// it is null, non-string or blank: it must not borrow a later signature.
func TestProdex04384GeminiNativeToolSignatureFirstPresentWins(t *testing.T) {
	for _, tc := range []struct {
		name       string
		part, call map[string]any
		want       string
	}{
		{"part-canonical", map[string]any{"thoughtSignature": "sig-part"}, map[string]any{"thoughtSignature": "sig-call"}, "sig-part"},
		{"nested-canonical", map[string]any{}, map[string]any{"thoughtSignature": "sig-call"}, "sig-call"},
		{"part-null-shadows-nested", map[string]any{"thoughtSignature": nil}, map[string]any{"thoughtSignature": "sig-call"}, ""},
		{"part-empty-shadows-nested", map[string]any{"thoughtSignature": ""}, map[string]any{"thoughtSignature": "sig-call"}, ""},
		{"part-blank-shadows-nested", map[string]any{"thoughtSignature": " \t"}, map[string]any{"thoughtSignature": "sig-call"}, ""},
		{"part-wrong-type-shadows-nested", map[string]any{"thoughtSignature": 42}, map[string]any{"thoughtSignature": "sig-call"}, ""},
		{"nested-null-does-not-fallback-alias", map[string]any{}, map[string]any{"thoughtSignature": nil, "thought_signature": "alias"}, ""},
		{"part-snake-alias-ignored", map[string]any{"thought_signature": "alias"}, map[string]any{}, ""},
		{"nested-snake-alias-ignored", map[string]any{}, map[string]any{"thought_signature": "alias"}, ""},
		{"part-snake-alias-does-not-shadow-camel-nested", map[string]any{"thought_signature": "alias"}, map[string]any{"thoughtSignature": "sig-call"}, "sig-call"},
		{"part-ASCII-control-is-not-Mojo-whitespace", map[string]any{"thoughtSignature": "\x1c"}, map[string]any{"thoughtSignature": "sig-call"}, "\x1c"},
		{"part-Unicode-whitespace-is-blank", map[string]any{"thoughtSignature": "\u00a0\u2005"}, map[string]any{"thoughtSignature": "sig-call"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := geminiNativeThoughtSignature(tc.part, tc.call)
			if got != tc.want {
				t.Fatalf("Gemini signature precedence: got=%q want=%q part=%#v call=%#v", got, tc.want, tc.part, tc.call)
			}
		})
	}
}

func TestProdex04384GeminiNativeToolSignaturePrecedenceInResponseItem(t *testing.T) {
	for _, tc := range []struct {
		name       string
		part, call map[string]any
		want       string
	}{
		{"valid-part", map[string]any{"thoughtSignature": "sig-part"}, map[string]any{"name": "read", "args": map[string]any{}, "thoughtSignature": "sig-nested"}, "sig-part"},
		{"null-part", map[string]any{"thoughtSignature": nil}, map[string]any{"name": "read", "args": map[string]any{}, "thoughtSignature": "sig-nested"}, ""},
		{"blank-part", map[string]any{"thoughtSignature": " "}, map[string]any{"name": "read", "args": map[string]any{}, "thoughtSignature": "sig-nested"}, ""},
		{"snake-only", map[string]any{"thought_signature": "alias"}, map[string]any{"name": "read", "args": map[string]any{}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := geminiNativeToolCallItem(tc.part, tc.call, "call-1")
			got, _ := item["gemini_thought_signature"].(string)
			if got != tc.want {
				t.Fatalf("Gemini response tool item signature=%q want=%q body=%#v", got, tc.want, item)
			}
			if _, present := item["gemini_thought_signature"]; present != (tc.want != "") {
				t.Fatalf("Gemini tool signature key presence=%t want=%t item=%#v", present, tc.want != "", item)
			}
		})
	}
}

// The rejected fallback must not leak into the complete Responses tool
// history; an invented signature could later be trusted as part of a
// model continuation even though the upstream part explicitly shadowed it.
func TestProdex04384GeminiNativeToolSignatureDoesNotLeakIntoResponseHistory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		parent any
		nested string
		want   bool
	}{
		{"canonical-part", "sig-parent", "sig-nested", true},
		{"missing-part", nil, "sig-nested", true},
		{"explicit-null-parent", map[string]any{"thoughtSignature": nil}, "sig-nested", false},
		{"explicit-blank-parent", map[string]any{"thoughtSignature": ""}, "sig-nested", false},
		{"nonstring-parent", map[string]any{"thoughtSignature": 7}, "sig-nested", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := map[string]any{"functionCall": map[string]any{"name": "read", "args": map[string]any{}, "thoughtSignature": tc.nested}}
			if tc.name == "canonical-part" {
				part["thoughtSignature"] = tc.parent
			}
			if alternate, ok := tc.parent.(map[string]any); ok {
				for key, value := range alternate {
					part[key] = value
				}
			}
			root := map[string]any{"responseId": "resp-tools", "modelVersion": "gemini-3.1-pro-preview", "candidates": []any{
				map[string]any{"content": map[string]any{"role": "model", "parts": []any{part}}, "finishReason": "STOP"},
			}}
			result := geminiNativeResponsesValue(root, nil, 1)
			output, ok := result["output"].([]any)
			if !ok || len(output) != 1 {
				t.Fatalf("native Gemini tool response output=%#v", result)
			}
			call, _ := output[0].(map[string]any)
			value, present := call["gemini_thought_signature"]
			if present != tc.want {
				t.Fatalf("Gemini response leaked shadowed signature=%v present=%t want=%t output=%#v", value, present, tc.want, call)
			}
			if tc.name == "canonical-part" && value != "sig-parent" {
				t.Fatalf("canonical parent signature lost: %#v", call)
			}
			if tc.name == "missing-part" && value != "sig-nested" {
				t.Fatalf("eligible nested signature lost: %#v", call)
			}
		})
	}
}
