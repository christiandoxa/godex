package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

func geminiBufferedArgsFixture04384(scenario string) (member, want string, ok bool) {
	switch scenario {
	case "buffered-args-null", "buffered-args-wrapped-null":
		return `,"args":null`, "null", true
	case "buffered-args-absent":
		return "", "{}", true
	case "buffered-args-empty":
		return `,"args":{}`, "{}", true
	case "buffered-args-canonical-wire", "buffered-args-patch-fallback":
		return `,"args":{"markup":"<tag>&\u2028\u2029","fraction":1e-5}`, "{\"fraction\":0.00001,\"markup\":\"<tag>&\u2028\u2029\"}", true
	}
	return "", "", false
}

func geminiBufferedArgsResponse04384(scenario string) (string, bool) {
	member, _, ok := geminiBufferedArgsFixture04384(scenario)
	if !ok {
		return "", false
	}
	name, metadata := "read_file", ""
	if scenario == "buffered-args-patch-fallback" {
		name = "apply_patch"
	}
	if scenario == "buffered-args-canonical-wire" {
		metadata = `,"avgLogprobs":1.000000,"logprobsResult":{"negativeZero":-0}`
	}
	response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"},{"functionCall":{"name":"` + name + `","id":"call-args"` + member + `}}]},"finishReason":"STOP"` + metadata + `}],"modelVersion":"gemini-3.1-pro-preview","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
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
	expected := map[string]any{
		"type": "function_call", "name": "read_file", "call_id": "call-args", "arguments": want,
	}
	if scenario == "buffered-args-patch-fallback" {
		expected = map[string]any{"type": "custom_tool_call", "name": "apply_patch", "call_id": "call-args", "input": want}
	}
	if !reflect.DeepEqual(item, expected) {
		return errors.New("Gemini buffered arguments changed field presence or tool identity")
	}
	return nil
}

func geminiBufferedArgsNegativeControl04384() bool {
	for _, scenario := range []string{"buffered-args-null", "buffered-args-wrapped-null", "buffered-args-absent", "buffered-args-empty", "buffered-args-canonical-wire", "buffered-args-patch-fallback"} {
		_, want, _ := geminiBufferedArgsFixture04384(scenario)
		item := map[string]any{"type": "function_call", "name": "read_file", "call_id": "call-args", "arguments": want}
		field := "arguments"
		if scenario == "buffered-args-patch-fallback" {
			field = "input"
			item = map[string]any{"type": "custom_tool_call", "name": "apply_patch", "call_id": "call-args", "input": want}
		}
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
			item[field] = value
			if value != want && geminiBufferedArgsInvariant04384(scenario, body) == nil {
				return false
			}
		}
	}
	return geminiBufferedWireNegativeControl04384()
}

// Keep every original response byte except the independently validated UUID.
func geminiBufferedWireProjection04384(body string) (string, error) {
	var root struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal([]byte(body), &root) != nil || len(root.ID) == 0 {
		return "", errors.New("Gemini buffered wire lacks response identity")
	}
	field := `"id":` + string(root.ID)
	if strings.Count(body, field) != 1 {
		return "", errors.New("Gemini buffered wire lacks a unique compact identity")
	}
	return strings.Replace(body, field, `"id":"response-uuid"`, 1), nil
}

func geminiBufferedWireNegativeControl04384() bool {
	canonical := "{\"id\":\"response-one\",\"fraction\":0.00001,\"markup\":\"<tag>&\u2028\u2029\"}"
	want, err := geminiBufferedWireProjection04384(canonical)
	other, otherErr := geminiBufferedWireProjection04384(strings.Replace(canonical, "response-one", "response-two", 1))
	if err != nil || otherErr != nil || want != other {
		return false
	}
	for _, mutated := range []string{
		strings.Replace(canonical, "<tag>", `\u003ctag\u003e`, 1),
		strings.Replace(canonical, "\u2028", `\u2028`, 1),
		strings.Replace(canonical, "0.00001", "1e-5", 1),
	} {
		got, err := geminiBufferedWireProjection04384(mutated)
		if err == nil && got == want {
			return false
		}
	}
	return true
}
