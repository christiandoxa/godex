package main

import (
	"errors"
	"reflect"
)

func geminiEnvelopeFixture04384(scenario string) (string, bool) {
	var value string
	switch scenario {
	case "buffered-envelope-null":
		value = "null"
	case "buffered-envelope-array":
		value = "[{}]"
	case "buffered-envelope-scalar":
		value = `"inner"`
	case "buffered-value-array":
		return "[{}]", true
	case "buffered-value-number":
		return "7", true
	default:
		return "", false
	}
	return `{"response":` + value + `,"traceId":"outer-trace","responseId":"outer-id","modelVersion":"outer-model","candidates":[{"content":{"parts":[{"text":"outer-must-not-leak"}]}}],"usageMetadata":{"promptTokenCount":99}}`, true
}

func geminiEnvelopeInvariant04384(body map[string]any) error {
	if err := geminiBufferedIdentityInvariant043711(body); err != nil {
		return err
	}
	expected := map[string]any{
		"id": body["id"], "object": "response", "model": "gemini-2.5-pro",
		"output": []any{}, "usage": map[string]any{}, "metadata": map[string]any{},
		"status": "failed", "error": map[string]any{
			"code": "gemini_empty_response", "message": "Gemini returned no visible response content.",
		},
	}
	if !reflect.DeepEqual(body, expected) {
		return errors.New("Gemini non-object envelope borrowed outer fields or changed empty-response semantics")
	}
	return nil
}
