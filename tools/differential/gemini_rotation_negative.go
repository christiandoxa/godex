package main

import (
	"encoding/json"
	"net/http"
)

var geminiQuotaModels043711 = []string{
	"gemini-3.1-pro-preview", "gemini-3.8-flash",
	"gemini-3.7-flash", "gemini-3.6-flash",
	"gemini-2.5-pro", "gemini-3.1-pro-preview",
}

// A fully independent miniature oracle fixture. Both products could
// otherwise accidentally make the same bad retry decision and still compare
// equal. The tagged 0.437.11 model/key order is an asserted contract.
func geminiQuotaControlRun043711(structured bool) geminiCandidateResult {
	scenarioModels := []string{geminiQuotaModels043711[0]}
	run := geminiCandidateResult{
		Name: "synthetic", ExitCode: 2,
		Client: exchange{Status: http.StatusTooManyRequests,
			Body: "Quota exhausted for this account\n"},
	}
	if structured {
		scenarioModels = geminiQuotaModels043711
		run.ExitCode = 0
		run.Client = exchange{
			Status: http.StatusOK,
			Body:   `{"id":"resp_deepseek_01900000-0000-7000-8000-000000000001","object":"response","model":"gemini-3.1-pro-preview","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"synthetic-ok"}]}],"metadata":{"gemini":{"finishReason":"STOP"}},"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`,
		}
	}
	for index, model := range scenarioModels {
		slot := "primary"
		if structured && index == len(scenarioModels)-1 {
			slot = "secondary"
		}
		run.Upstream = append(run.Upstream, geminiCapturedRequest{
			Method: http.MethodPost, Path: "/v1beta/openai/chat/completions",
			Slot: slot, Auth: true,
			Body: map[string]any{
				"model": model, "stream": false,
				"messages": []any{map[string]any{
					"role": "user", "content": "same request",
				}},
			},
		})
	}
	return run
}

func geminiQuotaClone043711(source geminiCandidateResult) (geminiCandidateResult, bool) {
	payload, err := json.Marshal(source)
	if err != nil {
		return geminiCandidateResult{}, false
	}
	var clone geminiCandidateResult
	if err := json.Unmarshal(payload, &clone); err != nil {
		return geminiCandidateResult{}, false
	}
	return clone, true
}

func geminiQuotaNegativeControl043711() bool {
	for _, scenario := range []struct {
		name       string
		structured bool
		mutations  []func(*geminiCandidateResult)
	}{
		{
			name: "quota-plain-429",
			mutations: []func(*geminiCandidateResult){
				func(r *geminiCandidateResult) { r.ExitCode = 0 },
				func(r *geminiCandidateResult) { r.Client.Status = http.StatusOK },
				func(r *geminiCandidateResult) { r.Client.Body = "Quota exhausted for this account" },
				func(r *geminiCandidateResult) { r.Upstream[0].Slot = "secondary" },
				func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
				func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			},
		},
		{
			name: "quota-structured-429", structured: true,
			mutations: []func(*geminiCandidateResult){
				func(r *geminiCandidateResult) { r.ExitCode = 2 },
				func(r *geminiCandidateResult) { r.Client.Status = http.StatusTooManyRequests },
				func(r *geminiCandidateResult) { r.Upstream = r.Upstream[:5] },
				func(r *geminiCandidateResult) { r.Upstream[1].Body["model"] = "gemini-3.7-flash" },
				func(r *geminiCandidateResult) { r.Upstream[3].Slot = "secondary" },
				func(r *geminiCandidateResult) { r.Upstream[5].Slot = "primary" },
				func(r *geminiCandidateResult) { r.Upstream[5].Auth = false },
				func(r *geminiCandidateResult) {
					r.Client.Body = `{"id":"resp_gemini_wrong","object":"response","model":"gemini-3.1-pro-preview","output":[]}`
				},
				func(r *geminiCandidateResult) {
					r.Client.Body = `{"id":"resp_deepseek_01900000-0000-7000-8000-000000000001","created_at":999,"object":"response","model":"gemini-3.1-pro-preview","output":[]}`
				},
				func(r *geminiCandidateResult) {
					var body map[string]any
					_ = json.Unmarshal([]byte(r.Client.Body), &body)
					body["usage"].(map[string]any)["input_tokens"] = float64(19)
					encoded, _ := json.Marshal(body)
					r.Client.Body = string(encoded)
				},
			},
		},
	} {
		good := geminiQuotaControlRun043711(scenario.structured)
		if err := geminiQuotaInvariant043711(scenario.name, good); err != nil {
			return false
		}
		for _, mutate := range scenario.mutations {
			bad, ok := geminiQuotaClone043711(good)
			if !ok {
				return false
			}
			mutate(&bad)
			if err := geminiQuotaInvariant043711(scenario.name, bad); err == nil {
				return false
			}
		}
	}
	return true
}
