package gemini

import (
	"encoding/json"
	"testing"
)

// Tagged gemini_response_media.mojo emits videoMetadata with
// js_raw_view(ParsedJson), where the tree comes from canonical serde_json
// 1.0.154. Go json.Marshal escapes HTML and U+2028/U+2029, and preserves
// raw json.Number spellings that canonical serde first converts to f64.
func TestProdex04384GeminiVideoMetadataUsesSerdeCanonicalJSON(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata any
		want     string
	}{
		{"HTML-is-not-escaped", map[string]any{"html": "<tag>&"}, `Gemini video metadata: {"html":"<tag>&"}`},
		{"Unicode-lines-not-escaped", map[string]any{"line": "\u2028\u2029"}, "Gemini video metadata: {\"line\":\"\u2028\u2029\"}"},
		{"escaped-control", map[string]any{"control": "\b\t\n"}, `Gemini video metadata: {"control":"\b\t\n"}`},
		{"escaped-backslash-unicode-literal", map[string]any{"literal": `\u2028`}, `Gemini video metadata: {"literal":"\\u2028"}`},
		{"canonical-decimals", map[string]any{"a": json.Number("1.00000000"), "b": json.Number("1e-5")}, `Gemini video metadata: {"a":1.0,"b":0.00001}`},
		{"serde-large-scientific", map[string]any{"large": json.Number("1e+20")}, `Gemini video metadata: {"large":1e+20}`},
		{"nested-and-sorted", map[string]any{"z": []any{map[string]any{"x": "<", "y": json.Number("1e-7")}}, "a": true}, `Gemini video metadata: {"a":true,"z":[{"x":"<","y":1e-7}]}`},
		{"explicit-null", nil, `Gemini video metadata: null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := map[string]any{"videoMetadata": tc.metadata}
			got := geminiSpecialPartText(part)
			if got != tc.want {
				t.Fatalf("video metadata case %s: got=%q want=%q", tc.name, got, tc.want)
			}
		})
	}
}

func TestProdex04384GeminiCanonicalVideoMetadataSurvivesCompleteResponse(t *testing.T) {
	part := map[string]any{"videoMetadata": map[string]any{"html": "<tag>&", "number": json.Number("1e-5")}}
	root := map[string]any{"responseId": "resp-video-canonical", "modelVersion": "gemini-3.1-pro-preview", "candidates": []any{
		map[string]any{"finishReason": "STOP", "content": map[string]any{"role": "model", "parts": []any{part}}},
	}}
	result := geminiNativeResponsesValue(root, nil, 1)
	output, ok := result["output"].([]any)
	if !ok || len(output) != 1 {
		t.Fatalf("Gemini response lost video text item: %#v", result)
	}
	message, _ := output[0].(map[string]any)
	content, _ := message["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("Gemini response content lost video text: %#v", message)
	}
	item, _ := content[0].(map[string]any)
	want := `Gemini video metadata: {"html":"<tag>&","number":0.00001}`
	if item["text"] != want {
		t.Fatalf("video response text=%q want=%q", item["text"], want)
	}
}
