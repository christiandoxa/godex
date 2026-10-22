package main

import (
	"encoding/json"
	"strings"
)

func geminiCompactOwnershipCases04384() []string {
	return []string{"compact-owner-previous", "compact-owner-session", "compact-owner-unbound", "compact-owner-assistant", "compact-owner-history-instructions", "compact-owner-duplicate", "compact-owner-numeric"}
}

func geminiCompactOwnershipSeed04384(scenario string) bool {
	return geminiCompactOwnershipName04384(scenario) && scenario != "compact-owner-unbound"
}

func geminiCompactOwnershipSession04384(scenario string) string {
	if scenario == "compact-owner-session" {
		return "synthetic-compact-session"
	}
	return ""
}

func geminiCompactOwnershipSeedInput04384(scenario string) string {
	body := `{"model":"gemini-3.1-pro-preview","input":"same request","stream":false`
	if scenario == "compact-owner-history-instructions" {
		body += `,"instructions":"synthetic seed instruction"`
	}
	if session := geminiCompactOwnershipSession04384(scenario); session != "" {
		body += `,"session_id":"` + session + `"`
	}
	return body + `}`
}

func geminiCompactOwnershipInput04384(scenario string) string {
	body := `{"model":"gemini-3.1-pro-preview","input":[]`
	switch scenario {
	case "compact-owner-previous", "compact-owner-assistant", "compact-owner-history-instructions", "compact-owner-duplicate", "compact-owner-numeric":
		body += `,"previous_response_id":"chatcmpl-differential"`
	case "compact-owner-session":
		body += `,"session_id":"synthetic-compact-session"`
	case "compact-owner-unbound":
		body += `,"previous_response_id":"chatcmpl-unknown"`
	}
	if scenario == "compact-owner-numeric" {
		body = strings.Replace(body, `"input":[]`, `"input":[{"type":"function_call_output","call_id":"call-synthetic","output":"done"}]`, 1)
	}
	if scenario == "compact-owner-duplicate" {
		body = strings.Replace(body, `"input":[]`, `"input":[{"type":"message","role":"user","content":"same request"}]`, 1)
	}
	return body + `}`
}

func geminiCompactOwnershipExpected04384(scenario string) geminiCandidateResult {
	if scenario == "compact-owner-unbound" {
		return geminiCompactExpected04384("compact-empty")
	}
	result := geminiCompactExpected04384("compact-empty")
	if scenario == "compact-owner-duplicate" {
		result.Client.Body = strings.Replace(result.Client.Body, `Semantic continuation summary:\n`, `Active user request that must still be completed:\nsame request\n\nSemantic continuation summary:\n`, 1)
		result.Stdout = result.Client.Body + "\n"
	}
	if scenario != "compact-owner-session" {
		request := &result.Upstream[0]
		messages := request.Body["messages"].([]any)
		history := []any{map[string]any{"role": "user", "content": "same request"}}
		if scenario == "compact-owner-history-instructions" {
			history = append([]any{map[string]any{"role": "system", "content": "synthetic seed instruction"}}, history...)
		}
		if scenario == "compact-owner-assistant" || scenario == "compact-owner-numeric" {
			assistant := map[string]any{"role": "assistant", "content": "synthetic cached assistant"}
			if scenario == "compact-owner-numeric" {
				assistant["content"] = ""
				assistant["tool_calls"] = []any{map[string]any{"id": "call-synthetic", "type": "function", "priority": json.Number("9007199254740993"), "function": map[string]any{"name": "lookup", "arguments": "{}"}}}
			}
			history = append(history, assistant)
			if scenario == "compact-owner-numeric" {
				history = append(history, map[string]any{"role": "tool", "tool_call_id": "call-synthetic", "content": "done"})
			}
		}
		request.Body["messages"] = append(append([]any{messages[0]}, history...), messages[1])
		wire, _ := json.Marshal(request.Body)
		request.RawBody = string(wire)
		_ = json.Unmarshal(wire, &request.Body)
	}
	seed := geminiUserIDExpected04384("request-user-id-absent")
	if scenario == "compact-owner-history-instructions" {
		seed.Upstream[0].Body["messages"] = []any{map[string]any{"role": "system", "content": "synthetic seed instruction"}, map[string]any{"role": "user", "content": "same request"}}
		wire, _ := json.Marshal(seed.Upstream[0].Body)
		seed.Upstream[0].RawBody = string(wire)
	}
	result.Client.Seed = &seed.Client
	result.Upstream = append(seed.Upstream, result.Upstream...)
	return result
}

func geminiCompactOwnershipName04384(scenario string) bool {
	return strings.HasPrefix(scenario, "compact-owner-")
}
