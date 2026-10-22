package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Reject equal-but-invalid fixture runs. Comparing two products alone can
// certify the same bug twice; these synthetic mutations must be observable
// through the independent wire and semantic-response guards.
func geminiNegativeControl() bool {
	if !geminiRawKeyIdentityNegativeControl04384() {
		return false
	}
	good := geminiCandidateResult{
		Name: "synthetic",
		Upstream: []geminiCapturedRequest{{
			Method: http.MethodPost, Path: "/v1beta/openai/chat/completions", Auth: true,
			Body: map[string]any{
				"model": "gemini-3.1-pro-preview", "stream": false,
				"messages": []any{map[string]any{"role": "user", "content": "same request"}},
			},
		}},
		Client: exchange{
			Status: http.StatusOK,
			Body:   `{"id":"resp_deepseek_01900000-0000-7000-8000-000000000001","object":"response","model":"gemini-3.1-pro-preview","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
		},
	}
	if err := geminiFixtureInvariant("chat-request", good); err != nil {
		return false
	}
	for _, modify := range []func(*geminiCandidateResult){
		func(run *geminiCandidateResult) {
			run.Upstream[0].Path = "/v1beta/models/gemini-3.1-pro-preview:generateContent"
		},
		func(run *geminiCandidateResult) { run.Upstream[0].Auth = false },
		func(run *geminiCandidateResult) { run.Upstream[0].Body["model"] = "unexpected" },
		func(run *geminiCandidateResult) { run.Client.Status = http.StatusInternalServerError },
		func(run *geminiCandidateResult) {
			run.Client.Body = `{"object":"response","model":"gemini-3.1-pro-preview","output":[]}`
		},
		func(run *geminiCandidateResult) {
			run.Client.Body = strings.Replace(run.Client.Body, "resp_deepseek_", "resp_gemini_", 1)
		},
		func(run *geminiCandidateResult) {
			run.Client.Body = strings.Replace(run.Client.Body, `"object":"response"`,
				`"created_at":999,"object":"response"`, 1)
		},
	} {
		mutated := good
		mutated.Upstream = append([]geminiCapturedRequest(nil), good.Upstream...)
		mutated.Upstream[0].Body = make(map[string]any, len(good.Upstream[0].Body))
		for key, value := range good.Upstream[0].Body {
			mutated.Upstream[0].Body[key] = value
		}
		modify(&mutated)
		if len(geminiCompare("chat-request", good, mutated)) == 0 {
			return false
		}
	}
	// An accepted Chat Completion body is a provider protocol failure, even
	// when its text appears plausible and the surrounding HTTP status is 200.
	if err := geminiFixtureInvariant("chat-response", good); err == nil {
		return false
	}
	// Valid UUIDv7 values are nondeterministic, while the absence of
	// created_at and the presence of material fields are strict contracts.
	second := good
	second.Client.Body = `{"id":"resp_deepseek_01900000-0000-7000-8000-000000000002","object":"response","model":"gemini-3.1-pro-preview","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
	if len(geminiCompare("chat-request", good, second)) != 0 {
		return false
	}
	var mutatedPayload map[string]any
	if err := json.Unmarshal([]byte(second.Client.Body), &mutatedPayload); err != nil {
		return false
	}
	mutatedPayload["usage"].(map[string]any)["input_tokens"] = float64(7)
	wire, err := json.Marshal(mutatedPayload)
	if err != nil {
		return false
	}
	second.Client.Body = string(wire)
	if len(geminiCompare("chat-request", good, second)) == 0 {
		return false
	}
	return geminiNativeSSESentinel043711() && geminiChatSSESentinel043711() &&
		geminiChatEOFSentinel04384() &&
		geminiChatErrorSentinel04384() &&
		geminiMultiTurnSentinel043711() && geminiQuotaNegativeControl043711() &&
		geminiBufferedArgsNegativeControl04384() && geminiBufferedBodyNegativeControl04384() && geminiContinuationNegativeControl04384() && geminiRequestNumberNegativeControl04384() && geminiUserIDNegativeControl04384() && geminiParallelNegativeControl04384() && geminiControlTypeNegativeControl04384()
}

// Ensure the executable oracle detects fabricated native Gemini SSE output
// and incorrect sequence numbers, not only request/body mismatches.
func geminiNativeSSESentinel043711() bool {
	wire := "event: response.created\r\ndata: " +
		"{\"created_at\":111,\"response\":{\"id\":\"resp-fixture\"},\"sequence_number\":0,\"type\":\"response.created\"}" +
		"\r\n\r\nevent: response.completed\r\ndata: " +
		"{\"created_at\":111,\"response\":{\"id\":\"resp-fixture\",\"output\":[]},\"sequence_number\":1,\"type\":\"response.completed\"}" +
		"\r\n\r\n"
	variant := strings.ReplaceAll(strings.ReplaceAll(wire, "resp-fixture", "resp-other"), "111", "999")
	if !geminiNativeSSEEqual043711(wire, variant) {
		return false
	}
	for _, modified := range []string{
		strings.Replace(wire, "\"sequence_number\":1", "\"sequence_number\":2", 1),
		strings.Replace(wire, "\"output\":[]", "\"output\":[{\"type\":\"message\"}]", 1),
		strings.Replace(wire, "event: response.completed", "event: response.failed", 1),
		strings.Split(wire, "event: response.completed")[0],
	} {
		if geminiNativeSSEEqual043711(wire, modified) {
			return false
		}
	}
	return true
}
