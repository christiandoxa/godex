package main

import (
	"encoding/json"
	"errors"
	"reflect"
)

type geminiToolIdentityFixture04384 struct {
	name, callID, namespace, short string
}

func geminiToolIdentityCase04384(scenario string) (geminiToolIdentityFixture04384, bool) {
	switch scenario {
	case "buffered-tool-dotted":
		return geminiToolIdentityFixture04384{"namespace.shell", "call-tool", "", "namespace.shell"}, true
	case "buffered-tool-padded":
		return geminiToolIdentityFixture04384{"  read_file  ", "call-tool", "", "  read_file  "}, true
	case "buffered-tool-empty":
		return geminiToolIdentityFixture04384{"", "", "", ""}, true
	case "buffered-tool-mcp-empty-prefix":
		return geminiToolIdentityFixture04384{"mcp____tool", "call-tool", "mcp__", "tool"}, true
	case "buffered-tool-trailing-separator":
		return geminiToolIdentityFixture04384{"namespace--sub--", "call-tool", "namespace", "sub--"}, true
	}
	return geminiToolIdentityFixture04384{}, false
}

func geminiToolIdentityResponse04384(scenario string) (string, bool) {
	fixture, ok := geminiToolIdentityCase04384(scenario)
	if !ok {
		return "", false
	}
	response := map[string]any{
		"modelVersion":  "gemini-3.1-pro-preview",
		"usageMetadata": map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 1, "totalTokenCount": 3},
		"candidates": []any{map[string]any{"finishReason": "STOP", "content": map[string]any{"parts": []any{
			map[string]any{"text": "synthetic-ok"},
			map[string]any{"functionCall": map[string]any{"name": fixture.name, "id": fixture.callID, "args": map[string]any{}}},
		}}}},
	}
	encoded, err := json.Marshal(response)
	return string(encoded), err == nil
}

func geminiToolIdentityInvariant04384(scenario string, body map[string]any) error {
	fixture, ok := geminiToolIdentityCase04384(scenario)
	output, _ := body["output"].([]any)
	if !ok || len(output) != 2 {
		return errors.New("Gemini tool identity output missing or duplicated")
	}
	want := map[string]any{"type": "function_call", "call_id": fixture.callID, "name": fixture.short, "arguments": "{}"}
	if fixture.namespace != "" {
		want["namespace"] = fixture.namespace
	}
	if !reflect.DeepEqual(output[1], want) {
		return errors.New("Gemini raw tool identity or namespace changed")
	}
	return nil
}
