package main

import (
	"encoding/json"
	"errors"
	"reflect"
)

func geminiPatchFixture04384(scenario string) (input, want string, ok bool) {
	switch scenario {
	case "buffered-patch-unified-update":
		return "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@ context\n-old\n+new\n\\ No newline at end of file",
			"*** Begin Patch\n*** Update File: file.txt\n@@ context\n-old\n+new\n*** End of File\n*** End Patch", true
	case "buffered-patch-unified-files":
		return "diff --git a/old.txt b/new.txt\n--- a/old.txt\n+++ b/new.txt\n@@ -1 +1 @@\n-old\n+new\ndiff --git a/add.txt b/add.txt\n--- /dev/null\n+++ b/add.txt\n@@ -0,0 +1 @@\n+added\ndiff --git a/gone.txt b/gone.txt\n--- a/gone.txt\n+++ /dev/null\n-gone",
			"*** Begin Patch\n*** Update File: old.txt\n*** Move to: new.txt\n@@\n-old\n+new\n*** Add File: add.txt\n+added\n*** Delete File: gone.txt\n*** End Patch", true
	case "buffered-patch-literal-crlf":
		return "literal\r\ntext\r", "literal\r\ntext\r", true
	}
	return "", "", false
}

func geminiPatchResponse04384(scenario string) (string, bool) {
	input, _, ok := geminiPatchFixture04384(scenario)
	if !ok {
		return "", false
	}
	response := map[string]any{
		"modelVersion":  "gemini-3.1-pro-preview",
		"usageMetadata": map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 1, "totalTokenCount": 3},
		"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"parts": []any{
			map[string]any{"text": "synthetic-ok"},
			map[string]any{"functionCall": map[string]any{"name": "apply_patch", "id": "call-patch", "args": map[string]any{"input": input}}},
		}}}},
	}
	encoded, err := json.Marshal(response)
	return string(encoded), err == nil
}

func geminiPatchInvariant04384(scenario string, body map[string]any) error {
	_, want, ok := geminiPatchFixture04384(scenario)
	output, _ := body["output"].([]any)
	if !ok || len(output) != 2 {
		return errors.New("Gemini patch output missing or duplicated")
	}
	if !reflect.DeepEqual(output[1], map[string]any{
		"type": "custom_tool_call", "name": "apply_patch", "call_id": "call-patch", "input": want,
	}) {
		return errors.New("Gemini unified-diff conversion or literal patch input changed")
	}
	return nil
}
