package main

import (
	"encoding/json"
	"net/http"
	"reflect"
)

func deepSeekHistoryCases04384() []string {
	return []string{"deepseek-history-extra", "deepseek-history-rtk", "deepseek-history-null", "deepseek-history-empty", "deepseek-history-numeric"}
}

func deepSeekHistoryCase04384(scenario string) bool {
	for _, name := range deepSeekHistoryCases04384() {
		if scenario == name {
			return true
		}
	}
	return false
}

func deepSeekHistoryInput04384(followup bool) []byte {
	if followup {
		return []byte(`{"model":"deepseek-v4-pro","input":[{"type":"function_call_output","call_id":"call-history","output":"synthetic result"}],"stream":false,"safety_identifier":"synthetic_user"}`)
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
	call := map[string]any{"id": "call-history", "type": "function", "function": map[string]any{"name": name, "arguments": arguments}}
	if scenario == "deepseek-history-numeric" {
		call["priority"] = json.Number("9007199254740993")
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
		"output":   []any{map[string]any{"type": "function_call", "call_id": "call-history", "name": function["name"], "arguments": function["arguments"]}},
		"usage":    map[string]any{"input_tokens": 2, "output_tokens": 1, "total_tokens": 3},
	})
	run.Client.Seed = &exchange{Status: 200, Body: string(body), Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}}
	user := map[string]any{"role": "user", "content": "history request"}
	assistant := map[string]any{"role": "assistant", "content": "", "tool_calls": []any{call}}
	tool := map[string]any{"role": "tool", "tool_call_id": "call-history", "content": "synthetic result"}
	run.Upstream = nil
	for index, messages := range [][]any{{user}, {user, assistant, tool}} {
		request := map[string]any{"model": "deepseek-v4-pro", "stream": false, "messages": messages}
		if index == 1 {
			request["user_id"] = "synthetic_user"
		}
		wire, _ := json.Marshal(request)
		run.Upstream = append(run.Upstream, upstreamRequest{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: string(wire)})
	}
	run.Retries = 1
	run.Events = []upstreamEvent{{Kind: "upstream.request", Attempt: 1}, {Kind: "upstream.response", Attempt: 1, Status: 200}, {Kind: "upstream.request", Attempt: 2}, {Kind: "upstream.response", Attempt: 2, Status: 200}}
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
		if !validSeed || run.ClientError != "" || len(run.StateIntegrity) != 0 || run.ExitStatus != 0 || run.Cancelled || run.Retries != 1 || run.Client.Status != 200 || run.Client.Body != want.Client.Body || run.Client.Headers.Get("Content-Type") != want.Client.Headers.Get("Content-Type") || run.Stdout != want.Stdout || !reflect.DeepEqual(run.Events, want.Events) || len(run.Upstream) != 2 {
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
