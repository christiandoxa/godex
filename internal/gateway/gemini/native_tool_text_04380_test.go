package gemini

import (
	"strings"
	"testing"
)

// The exact public Prodex 0.438.0 API-key Chat-compatible executable keeps
// visible text around `functionCall` key presence (including JSON null) and
// uses deterministic fallback `call_1`. The separate internal provider-core
// buffered normalizer suppresses text, but is NOT this public adapter.
func TestProdex04380GeminiAPIKeyBufferedKeepsTextAndNullToolCall(t *testing.T) {
	for _, tc := range []struct {
		name     string
		call     any
		wantName string
	}{
		{name: "null call still indicates function", call: nil, wantName: "tool_call"},
		{name: "concrete function", call: map[string]any{"name": "lookup", "id": "call-tagged", "args": map[string]any{"q": "synthetic"}}, wantName: "lookup"},
		{name: "string call remains tool signal", call: "malformed", wantName: "tool_call"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := map[string]any{"responseId": "resp_tool_text", "modelVersion": "gemini-3.1-pro-preview",
				"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{
					map[string]any{"text": "hidden before"},
					map[string]any{"functionCall": tc.call},
					map[string]any{"text": "hidden after"},
				}}}},
			}
			output := geminiNativeResponsesValueWithFallback04380(root, nil, 0, true)["output"].([]any)
			if len(output) != 2 {
				t.Fatalf("public API-key must preserve text AND function call, got %+v", output)
			}
			message, ok := output[0].(map[string]any)
			if !ok || message["type"] != "message" {
				t.Fatalf("visible text lost: %+v", output)
			}
			content, ok := message["content"].([]any)
			if !ok || len(content) != 1 || content[0].(map[string]any)["text"] != "hidden beforehidden after" {
				t.Fatalf("public visible text changed: %+v", content)
			}
			tool, ok := output[1].(map[string]any)
			if !ok || tool["type"] != "function_call" || tool["name"] != tc.wantName {
				t.Fatalf("function call lost or renamed: %+v", output)
			}
			expectedCallID := "call_1"
			if tc.wantName == "lookup" {
				expectedCallID = "call-tagged"
			}
			if tool["call_id"] != expectedCallID || strings.Contains(tool["arguments"].(string), "hidden") {
				t.Fatalf("wrong public API-key tool identity or leaked text: %+v", tool)
			}
		})
	}
}

func TestProdex04380GeminiBufferedPlainTextStaysVisibleWithoutFunctionCall(t *testing.T) {
	root := map[string]any{"responseId": "resp_plain", "candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": "visible"}}}}}}
	output := geminiNativeResponsesValue(root, nil, 0)["output"].([]any)
	if len(output) != 1 || output[0].(map[string]any)["type"] != "message" {
		t.Fatalf("plain-text response was lost: %+v", output)
	}
	content := output[0].(map[string]any)["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != "visible" {
		t.Fatalf("plain text unexpectedly suppressed: %+v", content)
	}
}

// The tagged direct generateContent/OpenAI-native adapter has a different
// contract from API-key Chat-compatible buffered normalization. Never apply
// the buffered tool-text suppression flag to that separate transport path.
func TestProdex04380GeminiNativeGenerateContentKeepsTextAcrossToolCall(t *testing.T) {
	root := map[string]any{"responseId": "resp_native", "candidates": []any{map[string]any{
		"content": map[string]any{"parts": []any{
			map[string]any{"text": "native visible"},
			map[string]any{"functionCall": map[string]any{"id": "call-native", "name": "lookup", "args": map[string]any{}}},
		}},
	}}}
	output := geminiNativeResponsesValue(root, nil, 0)["output"].([]any)
	if len(output) != 2 || output[0].(map[string]any)["type"] != "message" ||
		output[1].(map[string]any)["type"] != "function_call" {
		t.Fatalf("native/OAuth tool/text projection was changed by API-key suppression: %+v", output)
	}
	text := output[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if text != "native visible" {
		t.Fatalf("native transport lost visible text: %+v", output)
	}
}
