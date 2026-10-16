package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The public API-key Chat-compatible bridge keeps text around a null call,
// unlike the internal provider-core buffered normalizer used elsewhere.
// The native response ID is generated; the fallback function-call ID is the
// exact tagged `call_1`, not a fresh UUID. Everything else compares strictly.
func geminiBufferedNullToolProjection04380(body string) (map[string]any, error) {
	var root map[string]any
	if json.Unmarshal([]byte(body), &root) != nil || root == nil {
		return nil, errors.New("Gemini buffered null-tool response not an object")
	}
	output, ok := root["output"].([]any)
	if !ok || len(output) != 2 {
		return nil, fmt.Errorf("Gemini public null tool must emit text and tool item: %+v", output)
	}
	message, ok := output[0].(map[string]any)
	if !ok || message["type"] != "message" || message["role"] != "assistant" {
		return nil, fmt.Errorf("Gemini null-tool visible text item missing: %+v", output)
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 1 || content[0] == nil {
		return nil, fmt.Errorf("Gemini null-tool visible text content missing: %+v", output)
	}
	text, ok := content[0].(map[string]any)
	if !ok || text["type"] != "output_text" || text["text"] != "hidden beforehidden after" {
		return nil, fmt.Errorf("Gemini public null-tool text altered or removed: %+v", content)
	}
	call, ok := output[1].(map[string]any)
	if !ok || call["type"] != "function_call" || call["name"] != "tool_call" {
		return nil, fmt.Errorf("Gemini null functionCall lost fallback function_call item: %+v", output)
	}
	if call["call_id"] != "call_1" || call["arguments"] != "{}" {
		return nil, fmt.Errorf("Gemini null-tool ID/arguments differ: %+v", call)
	}
	responseID, _ := root["id"].(string)
	if !strings.HasPrefix(responseID, "resp_deepseek_") || len(responseID) <= len("resp_deepseek_") {
		return nil, fmt.Errorf("Gemini response identity missing: %q", responseID)
	}
	delete(root, "id")
	delete(root, "created_at")
	return root, nil
}
