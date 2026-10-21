package gemini

import (
	"strings"
	"testing"
)

func geminiIdentityGroundingRoot04384() map[string]any {
	return map[string]any{"candidates": []any{map[string]any{
		"finishReason": "STOP",
		"content":      map[string]any{"role": "model", "parts": []any{map[string]any{"text": "synthetic-ok"}}},
		"groundingMetadata": map[string]any{"groundingChunks": []any{map[string]any{
			"web": map[string]any{"uri": "https://example.test/source"},
		}}},
	}}}
}

func geminiWebSearchID04384(t *testing.T, response map[string]any) string {
	t.Helper()
	output, ok := response["output"].([]any)
	if !ok {
		t.Fatalf("invalid response output: %#v", response)
	}
	for _, raw := range output {
		item, ok := raw.(map[string]any)
		if ok && item["type"] == "web_search_call" {
			id, _ := item["id"].(string)
			return id
		}
	}
	t.Fatalf("missing synthetic grounding web_search_call: %#v", response)
	return ""
}

// Source Gemini provider response.rs has TWO distinct ID policies:
// API-key Chat-compatible `gemini_responses_value_from_generate_value`
// uses responseId exclusively, defaulting to gemini_resp_prodex. Direct
// native Gemini prefers the first-present responseId then id, and uses its
// own fallback resp_gemini_N. Both treat an explicit empty string as valid.
func TestProdex04384GeminiAPIKeyGroundingIdentityMatchesProviderTransform(t *testing.T) {
	for _, tc := range []struct {
		name              string
		fields            map[string]any
		wantID, wantModel string
	}{
		{"missing-response-id", map[string]any{}, "ws_gemini_resp_prodex", "gemini-2.5-pro"},
		{"unrelated-id-does-not-override", map[string]any{"id": "stale-native-id", "model": "stale-model"}, "ws_gemini_resp_prodex", "gemini-2.5-pro"},
		{"typed-response-id", map[string]any{"responseId": "response-provider", "modelVersion": "gemini-3.1-pro-preview"}, "ws_response-provider", "gemini-3.1-pro-preview"},
		{"null-response-id-does-not-borrow-id", map[string]any{"responseId": nil, "id": "stale"}, "ws_gemini_resp_prodex", "gemini-2.5-pro"},
		{"empty-response-id-preserved", map[string]any{"responseId": "", "id": "stale"}, "ws_", "gemini-2.5-pro"},
		{"empty-model-version-preserved", map[string]any{"modelVersion": "", "model": "stale"}, "ws_gemini_resp_prodex", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := geminiIdentityGroundingRoot04384()
			for key, value := range tc.fields {
				root[key] = value
			}
			response := geminiNativeResponsesValueWithFallback04380(root, nil, 1, true)
			id := geminiWebSearchID04384(t, response)
			if id != tc.wantID || response["model"] != tc.wantModel {
				t.Fatalf("chat-compatible identity input=%#v got grounding=%q model=%#v want %q / %q", tc.fields, id, response["model"], tc.wantID, tc.wantModel)
			}
		})
	}
}

func TestProdex04384GeminiDirectGroundingIdentityPreservesFirstPresent(t *testing.T) {
	for _, tc := range []struct {
		name              string
		fields            map[string]any
		wantID, wantModel string
		dynamic           bool
	}{
		{"missing-ids", map[string]any{}, "ws_resp_gemini_", "auto", true},
		{"id-alias", map[string]any{"id": "response-alias"}, "ws_response-alias", "auto", false},
		{"null-primary-shadows-id-alias", map[string]any{"responseId": nil, "id": "stale"}, "ws_resp_gemini_", "auto", true},
		{"empty-primary-shadows-id-alias", map[string]any{"responseId": "", "id": "stale"}, "ws_", "auto", false},
		{"null-model-version-shadows-model", map[string]any{"modelVersion": nil, "model": "stale"}, "ws_resp_gemini_", "auto", true},
		{"empty-model-version-shadows-model", map[string]any{"modelVersion": "", "model": "stale"}, "ws_resp_gemini_", "", true},
		{"model-alias-if-primary-absent", map[string]any{"model": "gemini-model-alias"}, "ws_resp_gemini_", "gemini-model-alias", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := geminiIdentityGroundingRoot04384()
			for key, value := range tc.fields {
				root[key] = value
			}
			response := geminiNativeResponsesValueWithFallback04380(root, nil, 1, false)
			id := geminiWebSearchID04384(t, response)
			match := id == tc.wantID
			if tc.dynamic {
				match = strings.HasPrefix(id, tc.wantID) && len(id) > len(tc.wantID)
			}
			if !match || response["model"] != tc.wantModel {
				t.Fatalf("native identity input=%#v got grounding=%q model=%#v want=%q dynamic=%t model=%q", tc.fields, id, response["model"], tc.wantID, tc.dynamic, tc.wantModel)
			}
		})
	}
}
