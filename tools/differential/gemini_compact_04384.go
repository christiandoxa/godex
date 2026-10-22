package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
)

const geminiCompactInstructions04384 = "Compact the supplied coding-agent transcript into one durable continuation summary. Preserve the user's goals, repository instructions, decisions, files changed, exact identifiers, commands and test results, unresolved failures, current worktree state, and the next concrete steps. Remove redundant narration and obsolete intermediate reasoning. Do not call tools. Return only the continuation summary, with no preamble or completion claim."
const geminiCompactPrefix04384 = "Another language model started to solve this problem and produced a summary of its thinking process. You also have access to the state of the tools that were used by that language model. Use this to build on the work that has already been done and avoid duplicating work. Here is the summary produced by the other language model, use the information in this summary to assist with your own analysis:"

func geminiCompactCases04384() []string {
	return append([]string{"compact-empty", "compact-active", "compact-controls", "compact-chat", "compact-content", "compact-responses", "compact-wrapped", "compact-wrapped-no-trace", "compact-fallback-empty", "compact-fallback-http", "compact-invalid-input", "compact-thought", "compact-limit-under", "compact-limit-at", "compact-limit-over", "compact-control-responses"}, geminiCompactOwnershipCases04384()...)
}

func geminiCompactCase04384(scenario string) bool {
	return slices.Contains(geminiCompactCases04384(), scenario)
}

func geminiCompactInput04384(scenario string) string {
	if geminiCompactOwnershipName04384(scenario) {
		return geminiCompactOwnershipInput04384(scenario)
	}
	if scenario == "compact-control-responses" {
		return `{"model":"gemini-3.1-pro-preview","input":"same request","stream":false}`
	}
	input, fields := `[]`, ""
	if scenario == "compact-active" {
		input = `[{"type":"message","role":"user","content":"continue synthetic task"}]`
	}
	if scenario == "compact-controls" {
		fields = `,"stream":true,"tools":null,"tool_choice":false,"include":null,"text":null,"instructions":"ignored instructions","max_output_tokens":80`
	}
	if scenario == "compact-invalid-input" {
		input = `"invalid"`
	}
	return `{"model":"gemini-3.1-pro-preview","input":` + input + fields + `}`
}

func geminiCompactUpstream04384(scenario string) (int, string) {
	status, body := 200, `{"candidates":[{"content":{"parts":[{"text":"synthetic-ok"}]}}]}`
	switch scenario {
	case "compact-chat":
		body = `{"choices":[{"message":{"content":" summary one "}},{"message":{"content":"ignored choice"}}],"output_text":"ignored output"}`
	case "compact-content":
		body = `{"choices":[{"message":{"content":[{"type":"other","text":" first "},"ignored",{"text":" second "},null]}}]}`
	case "compact-responses":
		body = `{"output":[{"type":"message","content":" one "},{"type":"message","content":[{"type":"other","text":" two "}]}],"output_text":"ignored output"}`
	case "compact-wrapped", "compact-wrapped-no-trace":
		body = `{"response":{"choices":[{"message":{"content":"inner summary"}}]}`
		if scenario == "compact-wrapped" {
			body += `,"traceId":""`
		}
		body += `}`
	case "compact-fallback-empty":
		body = `{}`
	case "compact-fallback-http":
		status, body = 500, `{"error":{"message":"synthetic provider error"}}`
	case "compact-thought":
		body = `{"candidates":[{"content":{"parts":[{"thought":true,"text":"ignored thought"},{"thought":false,"text":" visible "}]}},{"content":{"parts":[{"text":"ignored candidate"}]}}]}`
	case "compact-limit-under", "compact-limit-at", "compact-limit-over":
		limit := 4 << 20
		if scenario == "compact-limit-under" {
			limit--
		} else if scenario == "compact-limit-over" {
			limit++
		}
		body = `{"output_text":"bounded summary","padding":"` + strings.Repeat("x", limit-len(`{"output_text":"bounded summary","padding":""}`)) + `"}`
	}
	return status, body
}

func geminiCompactExpected04384(scenario string) geminiCandidateResult {
	if geminiCompactOwnershipName04384(scenario) {
		return geminiCompactOwnershipExpected04384(scenario)
	}
	if scenario == "compact-control-responses" {
		return geminiUserIDExpected04384("request-user-id-absent")
	}
	text, reason := "synthetic-ok", ""
	switch scenario {
	case "compact-chat":
		text = "summary one"
	case "compact-content":
		text = "first\nsecond"
	case "compact-responses":
		text = "one\ntwo"
	case "compact-wrapped":
		text = "inner summary"
	case "compact-thought":
		text = "visible"
	case "compact-limit-under", "compact-limit-at":
		text = "bounded summary"
	case "compact-wrapped-no-trace", "compact-fallback-empty", "compact-invalid-input":
		reason = "invalid-response"
	case "compact-fallback-http", "compact-limit-over":
		reason = "provider-error"
	}
	active := ""
	if scenario == "compact-active" {
		active = "Active user request that must still be completed:\ncontinue synthetic task\n\n"
	}
	text = active + "Semantic continuation summary:\n" + text + "\n\nContinue the active user request. Do not merely acknowledge repository, optimizer, or environment instructions."
	header := http.Header{"Content-Type": {"application/json; charset=utf-8"}, "X-Prodex-Compact-Mode": {"semantic"}, "X-Prodex-Compact-Provider": {"gemini"}}
	if reason != "" {
		text = "Local Prodex compact fallback summary.\n\nModel: gemini-3.1-pro-preview\nOriginal input items: 0\nRetained recent items: 0\n\nRecent conversation and tool state:\n- No parseable recent message or tool content was found."
		header.Set("X-Prodex-Compact-Mode", "local-fallback")
		header.Set("X-Prodex-Compact-Degraded", "true")
		header.Set("X-Prodex-Compact-Reason", reason)
	}
	quoted, _ := json.Marshal(geminiCompactPrefix04384 + "\n\n" + text)
	result := geminiCandidateResult{Client: exchange{Status: 200, Headers: header,
		Body: `{"output":[{"type":"message","role":"user","content":[{"type":"input_text","text":` + string(quoted) + `}]}]}`}}
	result.Stdout = result.Client.Body + "\n"
	if scenario != "compact-invalid-input" {
		messages := []any{map[string]any{"role": "system", "content": geminiCompactInstructions04384}}
		if scenario == "compact-active" {
			messages = append(messages, map[string]any{"role": "user", "content": "continue synthetic task"})
		}
		messages = append(messages, map[string]any{"role": "user", "content": geminiCompactInstructions04384})
		models := []string{"gemini-3.8-flash"}
		if scenario == "compact-fallback-http" {
			models = []string{"gemini-3.8-flash", "gemini-3.5-flash", "gemini-3-flash-preview", "gemini-2.5-flash"}
		}
		for _, model := range models {
			body := map[string]any{"messages": messages, "model": model, "stream": false}
			if scenario == "compact-controls" {
				body["max_tokens"] = 80
			}
			wire, _ := json.Marshal(body)
			request := geminiCapturedRequest{Method: "POST", Path: "/v1beta/openai/chat/completions", RawBody: string(wire), Auth: true, Slot: "single"}
			_ = json.Unmarshal(wire, &request.Body)
			result.Upstream = append(result.Upstream, request)
		}
	}
	return result
}

func geminiCompactInvariant04384(scenario string, result geminiCandidateResult) error {
	if geminiCompactOwnershipSeed04384(scenario) {
		seed := result.Client.Seed
		want := geminiUserIDExpected04384("request-user-id-absent").Client
		if seed == nil || seed.Seed != nil || seed.Status != want.Status || seed.Body != want.Body || seed.Headers.Get("Content-Type") != want.Headers.Get("Content-Type") {
			return errors.New("Gemini Compact lacks an exact successful owner seed")
		}
	}
	want := geminiCompactExpected04384(scenario)
	if !geminiExactRequestResult04384(result, want) {
		return errors.New("Gemini Compact lost original wire, selected model, raw summary, response, attempt or durable state")
	}
	for _, key := range []string{"X-Prodex-Compact-Mode", "X-Prodex-Compact-Provider", "X-Prodex-Compact-Degraded", "X-Prodex-Compact-Reason", "X-Upstream-Decoy"} {
		if result.Client.Headers.Get(key) != want.Client.Headers.Get(key) {
			return errors.New("Gemini Compact changed semantic/fallback headers")
		}
	}
	return nil
}
