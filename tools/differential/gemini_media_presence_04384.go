package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// These four fixtures exercise actual, published Prodex 0.439.0 executable
// response media behavior, beyond the local tagged-Mojo unit regressions.
// All upstream data, accounts, and API keys are synthetic loopback values.
func geminiMediaScenario04384(scenario string) bool {
	switch scenario {
	case "buffered-media-invalid-inline", "buffered-media-invalid-code",
		"buffered-media-suppressed-code", "buffered-grounding-null-status":
		return true
	}
	return false
}

func geminiMediaResponse04384(scenario string) string {
	parts := []any{map[string]any{"text": "synthetic-ok"}}
	candidate := map[string]any{"content": map[string]any{"role": "model", "parts": parts}, "finishReason": "STOP"}
	switch scenario {
	case "buffered-media-invalid-inline":
		parts = append(parts, map[string]any{"inlineData": nil,
			"fileData": map[string]any{"fileUri": "https://example.test/should-not-leak.png", "mimeType": "image/png"}})
	case "buffered-media-invalid-code":
		parts = append(parts, map[string]any{"executableCode": nil,
			"codeExecutionResult": map[string]any{"outcome": "OUTCOME_OK", "output": "must-not-leak"}})
	case "buffered-media-suppressed-code":
		parts = append(parts, map[string]any{"executableCode": map[string]any{"language": "", "code": "print(1)"}})
	case "buffered-grounding-null-status":
		candidate["groundingMetadata"] = map[string]any{"groundingChunks": []any{map[string]any{
			"web": map[string]any{"uri": "https://example.test/source", "urlRetrievalStatus": nil, "status": "FAILED"},
		}}}
	}
	response := map[string]any{
		"candidates":   []any{candidate},
		"modelVersion": "gemini-3.1-pro-preview",
		"usageMetadata": map[string]any{
			"promptTokenCount": 2, "candidatesTokenCount": 1, "totalTokenCount": 3,
		},
	}
	encoded, _ := json.Marshal(response)
	return string(encoded)
}

func geminiMediaFixtureInvariant04384(scenario string, result map[string]any) error {
	output, _ := result["output"].([]any)
	if len(output) < 1 {
		return errors.New("Gemini media fixture lost canonical assistant output")
	}
	message, _ := output[0].(map[string]any)
	content, _ := message["content"].([]any)
	if len(content) == 0 {
		return errors.New("Gemini media fixture lost canonical assistant message")
	}
	switch scenario {
	case "buffered-media-invalid-inline", "buffered-media-invalid-code":
		if len(output) != 1 || len(content) != 1 {
			return fmt.Errorf("Gemini invalid earlier media field leaked later output: output=%#v", output)
		}
	case "buffered-media-suppressed-code":
		// API-key Chat-compatible Gemini provider core suppresses code media
		// even though the separate native GenerateContent projector exposes
		// explicit empty labels. This is confirmed on both pinned binaries.
		if len(output) != 1 || len(content) != 1 {
			return fmt.Errorf("Gemini Chat-compatible executable media was unexpectedly surfaced: output=%#v", output)
		}
	case "buffered-grounding-null-status":
		if len(output) != 2 {
			return fmt.Errorf("Gemini grounding output missing or duplicated: %#v", output)
		}
		call, _ := output[1].(map[string]any)
		if call["type"] != "web_search_call" || call["id"] != "ws_gemini_resp_prodex" {
			return fmt.Errorf("Gemini provider-transform grounding ID/shape differs: %#v", call)
		}
		action, _ := call["action"].(map[string]any)
		sources, _ := action["sources"].([]any)
		if len(sources) != 1 {
			return fmt.Errorf("Gemini grounding source cardinality differs: %#v", action)
		}
		source, _ := sources[0].(map[string]any)
		if source["url"] != "https://example.test/source" {
			return fmt.Errorf("Gemini grounding canonical source missing: %#v", source)
		}
		if _, leaked := source["status"]; leaked {
			return fmt.Errorf("Gemini grounding null primary borrowed later status: %#v", source)
		}
	}
	return nil
}
