package main

import (
	"encoding/json"
	"net/http"
)

// Reject equal-but-invalid fixture runs. Comparing two products alone can
// certify the same bug twice; these synthetic mutations must be observable
// through the independent wire and semantic-response guards.
func geminiNegativeControl() bool {
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
			Body:   `{"id":"resp-one","created_at":1,"object":"response","model":"gemini-3.1-pro-preview","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
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
	// Response IDs and observed times are nondeterministic, whereas the
	// visible text, failure classification, usage and metadata are material.
	second := good
	second.Client.Body = `{"created_at":999,"id":"resp-two","object":"response","model":"gemini-3.1-pro-preview","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
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
	return len(geminiCompare("chat-request", good, second)) != 0
}
