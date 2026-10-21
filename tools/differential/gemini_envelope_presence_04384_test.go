package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiEnvelopeOracleRejectsMutations(t *testing.T) {
	canonical := `{"id":"resp_deepseek_01900000-0000-7000-8000-000000000001","object":"response","model":"gemini-2.5-pro","output":[],"metadata":{},"usage":{},"status":"failed","error":{"code":"gemini_empty_response","message":"Gemini returned no visible response content."}}`
	var body map[string]any
	if json.Unmarshal([]byte(canonical), &body) != nil || geminiEnvelopeInvariant04384(body) != nil {
		t.Fatal("canonical empty-response control rejected")
	}
	for key, value := range map[string]any{
		"model": "outer-model", "output": []any{map[string]any{"type": "message"}},
		"status": "completed", "metadata": map[string]any{"outer": true},
		"usage": map[string]any{"input_tokens": 99}, "id": "outer-id",
		"error": map[string]any{"code": "wrong_error"}, "created_at": 1,
	} {
		original, present := body[key]
		body[key] = value
		if geminiEnvelopeInvariant04384(body) == nil {
			t.Errorf("envelope oracle accepted mutated %s", key)
		}
		if present {
			body[key] = original
		} else {
			delete(body, key)
		}
	}
	for _, scenario := range []string{"buffered-envelope-null", "buffered-envelope-array", "buffered-envelope-scalar", "buffered-value-array", "buffered-value-number"} {
		wire, ok := geminiEnvelopeFixture04384(scenario)
		if !ok || !json.Valid([]byte(wire)) {
			t.Errorf("invalid native envelope fixture %s", scenario)
		}
	}
	if _, ok := geminiEnvelopeFixture04384("chat-request"); ok {
		t.Fatal("unrelated scenario matched envelope fixture")
	}
}
