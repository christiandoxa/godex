package gemini

import "testing"

// Tagged 0.438.4 gemini_response_media.mojo media_put_content uses
// media_field, which selects the FIRST PRESENT key before checking its
// type. Present invalid inlineData/fileData must shadow lower priority
// alternate media, not silently adopt a valid fallback from another field.
func TestProdex04384GeminiMediaFirstPresentBranchBlocksFallback(t *testing.T) {
	validFile := map[string]any{"fileUri": "https://example.test/photo.png", "mimeType": "image/png"}
	validInline := map[string]any{"mimeType": "image/png", "data": "QUJD"}
	for _, tc := range []struct {
		name string
		part map[string]any
		want string
	}{
		{"valid-file-control", map[string]any{"fileData": validFile}, "https://example.test/photo.png"},
		{"valid-inline-control", map[string]any{"inlineData": validInline}, "data:image/png;base64,QUJD"},
		{"present-null-inline-blocks-file", map[string]any{"inlineData": nil, "fileData": validFile}, ""},
		{"present-number-inline-blocks-file", map[string]any{"inlineData": 7, "fileData": validFile}, ""},
		{"present-null-inline-blocks-alias", map[string]any{"inlineData": nil, "inline_data": validInline, "fileData": validFile}, ""},
		{"present-wrong-inline-blocks-text", map[string]any{"inlineData": "not-object", "text": "data:image/png;base64,AAAA"}, ""},
		{"present-null-file-blocks-text", map[string]any{"fileData": nil, "text": "data:image/png;base64,AAAA"}, ""},
		{"present-array-file-blocks-text", map[string]any{"fileData": []any{}, "text": "data:image/png;base64,AAAA"}, ""},
		{"present-null-file-blocks-alias", map[string]any{"fileData": nil, "file_data": validFile}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := geminiMediaContentItem(tc.part)
			if tc.want == "" {
				if item != nil {
					t.Fatalf("present invalid higher-priority media field leaked lower-priority content: %+v", item)
				}
				return
			}
			if item == nil || item["image_url"] != tc.want {
				t.Fatalf("valid source media was lost: got=%+v want=%q", item, tc.want)
			}
		})
	}
}

func TestProdex04384GeminiSpecialMediaFirstPresentBranchBlocksFallback(t *testing.T) {
	validResult := map[string]any{"outcome": "OUTCOME_OK", "output": "test"}
	for _, tc := range []struct {
		name string
		part map[string]any
		want string
	}{
		{"code-valid-control", map[string]any{"executableCode": map[string]any{"language": "python", "code": "print(1)"}, "codeExecutionResult": validResult}, "Gemini executable code (python):\n```python\nprint(1)\n```"},
		{"code-null-blocks-result", map[string]any{"executableCode": nil, "codeExecutionResult": validResult}, ""},
		{"code-scalar-blocks-result", map[string]any{"executableCode": false, "codeExecutionResult": validResult}, ""},
		{"blank-code-blocks-result", map[string]any{"executableCode": map[string]any{"code": " \n "}, "codeExecutionResult": validResult}, ""},
		{"result-valid-control", map[string]any{"codeExecutionResult": validResult}, "Gemini code execution result (OUTCOME_OK):\n```text\ntest\n```"},
		{"result-null-blocks-video", map[string]any{"codeExecutionResult": nil, "videoMetadata": map[string]any{"resolution": "1080p"}}, "Gemini code execution result (OUTCOME_UNSPECIFIED):\n```text\n\n```"},
		{"result-scalar-blocks-video", map[string]any{"codeExecutionResult": 9, "videoMetadata": map[string]any{"resolution": "1080p"}}, "Gemini code execution result (OUTCOME_UNSPECIFIED):\n```text\n\n```"},
		{"video-present-null", map[string]any{"videoMetadata": nil}, "Gemini video metadata: null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := geminiSpecialPartText(tc.part); got != tc.want {
				t.Fatalf("special Gemini media %s got=%q want=%q", tc.name, got, tc.want)
			}
		})
	}
}

// The presentation consequence is observable in the completed Responses
// envelope: an invalid earlier metadata field cannot surface a fabricated
// image or executable result from a lower-priority branch.
func TestProdex04384GeminiNativeMediaPrecedenceCannotLeakOutput(t *testing.T) {
	file := map[string]any{"fileUri": "https://example.test/should-not-leak.png", "mimeType": "image/png"}
	for _, tc := range []struct {
		name    string
		part    map[string]any
		visible bool
	}{
		{"inline-null-blocks-file", map[string]any{"inlineData": nil, "fileData": file}, false},
		{"file-null-blocks-data-url", map[string]any{"fileData": nil, "text": "data:image/png;base64,AAAA"}, false},
		{"executable-null-blocks-result", map[string]any{"executableCode": nil, "codeExecutionResult": map[string]any{"output": "leak"}}, false},
		{"executable-blank-blocks-result", map[string]any{"executableCode": map[string]any{"code": "\t "}, "codeExecutionResult": map[string]any{"output": "leak"}}, false},
		{"inline-valid", map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "QUJD"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := map[string]any{"responseId": "resp-media-policy", "modelVersion": "gemini-3.1-pro-preview", "candidates": []any{
				map[string]any{"finishReason": "STOP", "content": map[string]any{"role": "model", "parts": []any{tc.part}}},
			}}
			response := geminiNativeResponsesValue(root, nil, 1)
			output, _ := response["output"].([]any)
			if visible := len(output) > 0; visible != tc.visible {
				t.Fatalf("Gemini response leaked lower-priority media=%t want=%t output=%#v", visible, tc.visible, output)
			}
		})
	}
}
