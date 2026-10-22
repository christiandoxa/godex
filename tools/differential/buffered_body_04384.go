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
	if len(scenario.Runs) != 2 {
		return []string{scenario.Name + ".run_count"}
	}
	var failures []string
	for _, run := range scenario.Runs {
		prefix := scenario.Name + "." + run.Name
		if run.ExitStatus != 2 || run.Client.Status != 502 || run.Client.Body != want ||
			strings.TrimSuffix(run.Stdout, "\n") != want || run.Client.Headers.Get("Content-Type") != "text/plain; charset=utf-8" {
			failures = append(failures, prefix+".exact_oversize_rejection")
		}
		slot := "single"
		if scenario.Name == "deepseek-buffered-body-over-rotation" {
			slot = "primary"
		}
		failures = append(failures, bufferedToolAttemptInvariants04384(prefix, run, slot)...)
	}
	return failures
}
