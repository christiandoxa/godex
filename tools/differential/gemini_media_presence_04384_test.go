package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiMediaExecutableFixtureRejectsOutputMutations(t *testing.T) {
	for _, scenario := range []string{
		"buffered-media-invalid-inline", "buffered-media-invalid-code",
		"buffered-media-suppressed-code", "buffered-grounding-null-status",
	} {
		t.Run(scenario, func(t *testing.T) {
			var native map[string]any
			if err := json.Unmarshal([]byte(geminiMediaResponse04384(scenario)), &native); err != nil {
				t.Fatal(err)
			}
			if native["modelVersion"] != "gemini-3.1-pro-preview" {
				t.Fatal("synthetic native fixture model mutated")
			}
			output := []any{map[string]any{"type": "message", "role": "assistant", "content": []any{
				map[string]any{"type": "output_text", "text": "synthetic-ok"},
			}}}
			switch scenario {
			case "buffered-grounding-null-status":
				output = append(output, map[string]any{"type": "web_search_call", "id": "ws_gemini_resp_prodex", "action": map[string]any{
					"sources": []any{map[string]any{"type": "url", "url": "https://example.test/source"}},
				}})
			}
			body := map[string]any{"output": output}
			if err := geminiMediaFixtureInvariant04384(scenario, body); err != nil {
				t.Fatalf("valid media oracle rejected canonical fixture: %v", err)
			}
			// Matching Prodex/Godex outputs is insufficient without an
			// independent negative control detecting unwanted lower-priority
			// media or altered metadata in both responses.
			if scenario == "buffered-media-invalid-inline" || scenario == "buffered-media-invalid-code" {
				message := output[0].(map[string]any)
				message["content"] = append(message["content"].([]any), map[string]any{"type": "input_image", "image_url": "data:image/png;base64,AAAA"})
			} else if scenario == "buffered-media-suppressed-code" {
				message := output[0].(map[string]any)
				message["content"] = append(message["content"].([]any), map[string]any{
					"type": "output_text", "text": "Gemini executable code ():\n```\nprint(1)\n```",
				})
			} else {
				action := output[1].(map[string]any)["action"].(map[string]any)
				action["sources"].([]any)[0].(map[string]any)["status"] = "FAILED"
			}
			if err := geminiMediaFixtureInvariant04384(scenario, body); err == nil {
				t.Fatal("media oracle missed a deliberate semantic mutation")
			}
		})
	}
}
