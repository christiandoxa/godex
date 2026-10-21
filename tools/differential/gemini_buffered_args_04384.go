package main

import (
	"errors"
	"reflect"
)

func geminiBufferedArgsFixture04384(scenario string) (member, want string, ok bool) {
	switch scenario {
	case "buffered-args-null", "buffered-args-wrapped-null":
		return `,"args":null`, "null", true
	case "buffered-args-absent":
		return "", "{}", true
	case "buffered-args-empty":
		return `,"args":{}`, "{}", true
	}
	return "", "", false
}

func geminiBufferedArgsResponse04384(scenario string) (string, bool) {
	member, _, ok := geminiBufferedArgsFixture04384(scenario)
	if !ok {
		return "", false
	}
	response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"},{"functionCall":{"name":"read_file","id":"call-args"` + member + `}}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
	if scenario == "buffered-args-wrapped-null" {
		response = `{"traceId":"synthetic-args-trace","response":` + response + `}`
	}
	return response, true
}

func geminiBufferedArgsInvariant04384(scenario string, body map[string]any) error {
	_, want, ok := geminiBufferedArgsFixture04384(scenario)
	output, _ := body["output"].([]any)
	if !ok || len(output) != 2 {
		return errors.New("Gemini buffered arguments lost or duplicated output")
	}
	item, _ := output[1].(map[string]any)
	if !reflect.DeepEqual(item, map[string]any{
		"type": "function_call", "name": "read_file", "call_id": "call-args", "arguments": want,
	}) {
		return errors.New("Gemini buffered arguments changed field presence or tool identity")
	}
	return nil
}

func geminiBufferedArgsNegativeControl04384() bool {
	for _, scenario := range []string{"buffered-args-null", "buffered-args-wrapped-null", "buffered-args-absent", "buffered-args-empty"} {
		_, want, _ := geminiBufferedArgsFixture04384(scenario)
		item := map[string]any{"type": "function_call", "name": "read_file", "call_id": "call-args", "arguments": want}
		body := map[string]any{"output": []any{map[string]any{"type": "message"}, item}}
		if geminiBufferedArgsInvariant04384(scenario, body) != nil {
			return false
		}
		for key, value := range item {
			item[key] = "mutated"
			if geminiBufferedArgsInvariant04384(scenario, body) == nil {
				return false
			}
			item[key] = value
		}
		for _, value := range []any{nil, "{}", "null", map[string]any{}} {
			item["arguments"] = value
			if value != want && geminiBufferedArgsInvariant04384(scenario, body) == nil {
				return false
			}
		}
	}
	return true
}
