package main

import (
	"encoding/json"
	"strings"
)

func bufferedBodyCases04384() []string {
	return []string{"deepseek-buffered-body-under", "deepseek-buffered-body-limit", "deepseek-buffered-body-over", "deepseek-buffered-body-over-rotation"}
}

func bufferedBodyCase04384(phase string) (int, bool) {
	switch phase {
	case "deepseek-buffered-body-under":
		return (4 << 20) - 1, true
	case "deepseek-buffered-body-limit":
		return 4 << 20, true
	case "deepseek-buffered-body-over", "deepseek-buffered-body-over-rotation":
		return (4 << 20) + 1, true
	}
	return 0, false
}

func bufferedBodyUpstream04384(phase, body string) string {
	size, known := bufferedBodyCase04384(phase)
	if !known {
		return body
	}
	return paddedBufferedUpstream04384(body, size)
}

func paddedBufferedUpstream04384(body string, size int) string {
	var root map[string]any
	if json.Unmarshal([]byte(body), &root) != nil {
		return ""
	}
	root["padding"] = ""
	base, _ := json.Marshal(root)
	root["padding"] = strings.Repeat(" ", size-len(base))
	encoded, _ := json.Marshal(root)
	return string(encoded)
}

func bufferedBodyExpected04384(phase string) string {
	if size, known := bufferedBodyCase04384(phase); known && size > 4<<20 {
		return "provider response could not be processed"
	}
	return bufferedToolSuccessResponse04384(map[string]any{"type": "function_call", "name": "lookup",
		"call_id": "call-differential", "arguments": `{"key":"alpha"}`})
}

func bufferedBodyInvariants04384(scenario scenarioResult) []string {
	want := bufferedBodyExpected04384(scenario.Name)
	size, _ := bufferedBodyCase04384(scenario.Name)
	if size <= 4<<20 {
		return bufferedToolWireInvariants04384(scenario, want)
	}
	slot := "single"
	if scenario.Name == "deepseek-buffered-body-over-rotation" {
		slot = "primary"
	}
	return bufferedToolTerminalWireInvariants04384(scenario, 502, want, "text/plain; charset=utf-8", slot)
}
