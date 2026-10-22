package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
)

func deepSeekHistoryCases04384() []string {
	return []string{"deepseek-history-extra", "deepseek-history-rtk", "deepseek-history-null", "deepseek-history-empty", "deepseek-history-numeric", "deepseek-history-simple", "deepseek-history-simple-inline", "deepseek-history-simple-unbound", "deepseek-history-simple-pending", "deepseek-history-numeric-pending", "deepseek-history-u64-pending", "deepseek-history-id-padded", "deepseek-history-id-output-padded", "deepseek-history-id-call-padded", "deepseek-history-simple-padded-pending"}
}

func deepSeekHistoryCallIDs04384(scenario string) (string, string) {
	switch scenario {
	case "deepseek-history-id-padded", "deepseek-history-simple-padded-pending":
		return " call-history ", " call-history "
	case "deepseek-history-id-output-padded":
		return "call-history", " call-history "
	case "deepseek-history-id-call-padded":
		return " call-history ", "call-history"
	default:
		return "call-history", "call-history"
	}
}

func deepSeekHistoryPending04384(scenario string) bool {
	return strings.HasSuffix(scenario, "-pending")
}

func deepSeekHistoryCase04384(scenario string) bool {
	for _, name := range deepSeekHistoryCases04384() {
		if scenario == name {
			return true
		}
	}
	return false
}

func deepSeekHistoryInput04384(scenario string, phase int) []byte {
	if phase > 0 {
		body := `{"model":"deepseek-v4-pro","input":[{"type":"function_call_output","call_id":"call-history","output":"synthetic result"}],"stream":false}`
		if scenario == "deepseek-history-simple-unbound" {
			body = strings.Replace(body, "call-history", "call-unknown", 1)
		}
		if scenario == "deepseek-history-simple-inline" {
			body = `{"model":"deepseek-v4-pro","input":[{"type":"message","role":"user","content":"history request"},{"type":"function_call","call_id":"call-history","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call-history","output":"synthetic result"}],"stream":false}`
		}
		if phase == 2 || !strings.HasPrefix(scenario, "deepseek-history-simple") {
			body = strings.TrimSuffix(body, "}") + `,"safety_identifier":"synthetic_user"}`
		}
		if strings.HasPrefix(scenario, "deepseek-history-id-") {
			body = strings.Replace(body, `"output":"synthetic result"}]`, `"output":"synthetic result"},{"type":"message","role":"user","content":"current request"}]`, 1)
		}
		_, outputID := deepSeekHistoryCallIDs04384(scenario)
		body = strings.ReplaceAll(body, "call-history", outputID)
		return []byte(body)
	}
	return []byte(`{"model":"deepseek-v4-pro","input":"history request","stream":false}`)
}

func deepSeekHistoryCall04384(scenario string, wrapped bool) map[string]any {
	name, arguments := "lookup", "{}"
	if scenario == "deepseek-history-rtk" {
		name, arguments = "shell", `{"cmd":"ls"}`
		if wrapped {
			arguments = `{"cmd":"rtk ls"}`
		}
	}
	callID, _ := deepSeekHistoryCallIDs04384(scenario)
	call := map[string]any{"id": callID, "type": "function", "function": map[string]any{"name": name, "arguments": arguments}}
	if scenario == "deepseek-history-numeric" || scenario == "deepseek-history-numeric-pending" {
		call["priority"] = json.Number("9007199254740993")
	}
	if scenario == "deepseek-history-u64-pending" {
		call["priority"] = json.Number("18446744073709551615")
	}
	return call
}

func deepSeekHistorySeed04384(scenario string) string {
	var content any
	if scenario == "deepseek-history-empty" {
		content = ""
	}
	message := map[string]any{"role": "assistant", "content": content, "tool_calls": []any{deepSeekHistoryCall04384(scenario, false)}}
	if scenario == "deepseek-history-extra" {
		message["provider_decoy"] = map[string]any{"value": "must-not-replay"}
	}
	body, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-history-seed", "object": "chat.completion", "created": 1, "model": "deepseek-v4-pro",
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": "tool_calls"}},
		"usage":   map[string]any{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3},
	})
	return string(body)
}

func deepSeekHistoryExpected04384(scenario string) productRun {
	run := deepSeekPrimitiveExpected04384("deepseek-request-primitive-output")
	call := deepSeekHistoryCall04384(scenario, true)
	function := call["function"].(map[string]any)
	body, _ := json.Marshal(map[string]any{
		"created_at": 1, "id": "chatcmpl-history-seed", "model": "deepseek-v4-pro", "object": "response",
		"metadata": map[string]any{"deepseek": map[string]any{"finish_reason": "tool_calls"}},
		"output":   []any{map[string]any{"type": "function_call", "call_id": call["id"], "name": function["name"], "arguments": function["arguments"]}},
		"usage":    map[string]any{"input_tokens": 2, "output_tokens": 1, "total_tokens": 3},
	})
	run.Client.Seed = &exchange{Status: 200, Body: string(body), Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}}
	user := map[string]any{"role": "user", "content": "history request"}
	assistant := map[string]any{"role": "assistant", "content": "", "tool_calls": []any{call}}
	_, outputID := deepSeekHistoryCallIDs04384(scenario)
	tool := map[string]any{"role": "tool", "tool_call_id": outputID, "content": "synthetic result"}
	if scenario == "deepseek-history-simple-unbound" {
		tool["tool_call_id"] = "call-unknown"
	}
	requests := [][]any{{user}, {user, assistant, tool}}
	if strings.HasPrefix(scenario, "deepseek-history-id-") {
		if scenario != "deepseek-history-id-padded" {
			requests[1] = nil
		}
		requests[1] = append(requests[1], map[string]any{"role": "user", "content": "current request"})
	}
	if strings.HasPrefix(scenario, "deepseek-history-simple") && scenario != "deepseek-history-simple-inline" {
		requests[1] = []any{tool}
	}
	if deepSeekHistoryPending04384(scenario) {
		requests = append(requests, []any{user, assistant, tool, map[string]any{"role": "assistant", "content": "synthetic-ok"}})
	}
	run.Upstream = nil
	for index, messages := range requests {
		request := map[string]any{"model": "deepseek-v4-pro", "stream": false, "messages": messages}
		if index == 2 || index == 1 && !strings.HasPrefix(scenario, "deepseek-history-simple") {
			request["user_id"] = "synthetic_user"
		}
		wire, _ := json.Marshal(request)
		run.Upstream = append(run.Upstream, upstreamRequest{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: string(wire)})
	}
	run.Retries = 1
	run.Events = []upstreamEvent{{Kind: "upstream.request", Attempt: 1}, {Kind: "upstream.response", Attempt: 1, Status: 200}, {Kind: "upstream.request", Attempt: 2}, {Kind: "upstream.response", Attempt: 2, Status: 200}}
	if deepSeekHistoryPending04384(scenario) {
		run.Retries = 2
		run.Events = append(run.Events, upstreamEvent{Kind: "upstream.request", Attempt: 3}, upstreamEvent{Kind: "upstream.response", Attempt: 3, Status: 200})
	}
	return run
}

func deepSeekHistoryInvariants04384(scenario scenarioResult) []string {
	if len(scenario.Runs) != 2 {
		return []string{scenario.Name + ".run_count"}
	}
	want := deepSeekHistoryExpected04384(scenario.Name)
	var failures []string
	for _, run := range scenario.Runs {
		seed := run.Client.Seed
		validSeed := seed != nil && seed.Seed == nil && seed.Status == 200 && seed.Body == want.Client.Seed.Body && seed.Headers.Get("Content-Type") == want.Client.Seed.Headers.Get("Content-Type")
		if !validSeed || run.ClientError != "" || len(run.StateIntegrity) != 0 || run.ExitStatus != 0 || run.Cancelled || run.Retries != want.Retries || run.Client.Status != 200 || run.Client.Body != want.Client.Body || run.Client.Headers.Get("Content-Type") != want.Client.Headers.Get("Content-Type") || run.Stdout != want.Stdout || !reflect.DeepEqual(run.Events, want.Events) || len(run.Upstream) != len(want.Upstream) {
			failures = append(failures, scenario.Name+"."+run.Name+".seed_response_attempt_or_state")
			continue
		}
		for i, request := range run.Upstream {
			expected := want.Upstream[i]
			if request.Method != expected.Method || request.Path != expected.Path || !request.AuthOK || request.KeySlot != "single" || request.Body != expected.Body {
				failures = append(failures, scenario.Name+"."+run.Name+".exact_history_request")
			}
		}
	}
	return failures
}
