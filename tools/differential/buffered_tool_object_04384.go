package main

import (
	"encoding/json"
	"strings"
)

func bufferedToolObjectFixture04384(phase string) (name, arguments, diagnostic string) {
	switch phase {
	case "deepseek-buffered-tool-key":
		return "lookup", "{bad", "key must be a string at line 1 column 2"
	case "deepseek-buffered-tool-comma":
		return "lookup", `{"key":"alpha",}`, "trailing comma at line 1 column 16"
	case "deepseek-buffered-tool-colon":
		return "lookup", `{"key" "alpha"}`, "expected `:` at line 1 column 8"
	case "deepseek-buffered-tool-separator":
		return "lookup", `{"key":1 "other":2}`, "expected `,` or `}` at line 1 column 10"
	case "deepseek-buffered-tool-search-key":
		return "tool_search", "{bad", "key must be a string at line 1 column 2"
	case "deepseek-buffered-tool-padded-key":
		return "  lookup  ", "{bad", "key must be a string at line 1 column 2"
	}
	return "", "", ""
}

func bufferedToolObjectResponse04384(phase string) string {
	name, _, diagnostic := bufferedToolObjectFixture04384(phase)
	if name == "" {
		return ""
	}
	return bufferedToolFailedResponse04384(name, diagnostic)
}

func bufferedToolFailedResponse04384(name, diagnostic string) string {
	return bufferedToolFailedMessage04384("DeepSeek returned malformed JSON arguments for tool call `" + name + "`: " + diagnostic)
}

func bufferedToolFailedMessage04384(diagnostic string) string {
	message, _ := json.Marshal(diagnostic)
	return `{"created_at":1,"error":{"code":"invalid_tool_call_arguments","message":` + string(message) + `},"id":"chatcmpl-differential-tool","metadata":{"deepseek":{"finish_reason":"tool_calls"}},"model":"deepseek-v4-pro","object":"response","output":[],"status":"failed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
}

func bufferedToolSuccessResponse04384(item map[string]any) string {
	output, _ := json.Marshal([]any{item})
	return `{"created_at":1,"id":"chatcmpl-differential-tool","metadata":{"deepseek":{"finish_reason":"tool_calls"}},"model":"deepseek-v4-pro","object":"response","output":` + string(output) + `,"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
}

func bufferedToolObjectInvariants04384(scenario scenarioResult) []string {
	return bufferedToolWireInvariants04384(scenario, bufferedToolObjectResponse04384(scenario.Name))
}

func bufferedToolWireInvariants04384(scenario scenarioResult, want string) []string {
	if len(scenario.Runs) != 2 {
		return []string{scenario.Name + ".run_count"}
	}
	var failures []string
	for _, run := range scenario.Runs {
		prefix := scenario.Name + "." + run.Name
		if want == "" || run.ExitStatus != 0 || run.Client.Status != 200 || run.Client.Body != want ||
			strings.TrimSuffix(run.Stdout, "\n") != want || run.Client.Headers.Get("Content-Type") != "application/json; charset=utf-8" {
			failures = append(failures, prefix+".exact_failed_response")
		}
		failures = append(failures, bufferedToolAttemptInvariants04384(prefix, run, "single")...)
	}
	return failures
}

func bufferedToolAttemptInvariants04384(prefix string, run productRun, slot string) []string {
	var failures []string
	if len(run.Upstream) != 1 || run.Retries != 0 || len(run.StateIntegrity) != 0 {
		failures = append(failures, prefix+".attempts_or_state")
	}
	for _, request := range run.Upstream {
		if request.Method != "POST" || request.Path != "/v1/chat/completions" || !request.AuthOK ||
			request.KeySlot != slot || !validFixtureRequest(request.Body) {
			failures = append(failures, prefix+".request_or_credential")
		}
	}
	return failures
}

func bufferedToolTerminalWireInvariants04384(scenario scenarioResult, status int, body, contentType, slot string) []string {
	if len(scenario.Runs) != 2 {
		return []string{scenario.Name + ".run_count"}
	}
	var failures []string
	for _, run := range scenario.Runs {
		prefix := scenario.Name + "." + run.Name
		if run.ExitStatus != 2 || run.Client.Status != status || run.Client.Body != body ||
			run.Stdout != body+"\n" || run.Client.Headers.Get("Content-Type") != contentType ||
			(contentType == "" && len(run.Client.Headers.Values("Content-Type")) != 0) {
			failures = append(failures, prefix+".exact_terminal_response")
		}
		failures = append(failures, bufferedToolAttemptInvariants04384(prefix, run, slot)...)
	}
	return failures
}
