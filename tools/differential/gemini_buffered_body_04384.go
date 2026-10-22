package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

const geminiBufferedBodySuccess04384 = `{"id":"response-uuid","metadata":{"gemini":{"finishReason":"STOP","usageMetadata":{"candidatesTokenCount":1,"promptTokenCount":2,"totalTokenCount":3}}},"model":"gemini-3.1-pro-preview","object":"response","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"input_tokens_details":{"cached_tokens":0,"tool_tokens":0},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":3}}`

func geminiBufferedBodyCase04384(scenario string) (int, bool) {
	switch scenario {
	case "buffered-body-under":
		return (4 << 20) - 1, true
	case "buffered-body-limit":
		return 4 << 20, true
	case "buffered-body-over", "buffered-body-over-rotation":
		return (4 << 20) + 1, true
	}
	return 0, false
}

func geminiBufferedBodyResponse04384(scenario string) (string, bool) {
	size, known := geminiBufferedBodyCase04384(scenario)
	if !known {
		return "", false
	}
	const body = `{"candidates":[{"content":{"role":"model","parts":[{"text":"synthetic-ok"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview","usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
	return paddedBufferedUpstream04384(body, size), true
}

func geminiBufferedBodyInvariant04384(scenario string, result geminiCandidateResult) error {
	size, known := geminiBufferedBodyCase04384(scenario)
	if !known || result.Stdout != result.Client.Body+"\n" || len(result.StateIntegrity) != 0 || len(result.Upstream) != 1 {
		return errors.New("Gemini body boundary stdout, attempts or durable state mismatch")
	}
	slot := "single"
	if scenario == "buffered-body-over-rotation" {
		slot = "primary"
	}
	request := result.Upstream[0]
	want := map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
		"messages": []any{map[string]any{"role": "user", "content": "same request"}}}
	if request.Method != "POST" || request.Path != "/v1beta/openai/chat/completions" || !request.Auth || request.Slot != slot || !reflect.DeepEqual(request.Body, want) {
		return errors.New("Gemini body boundary changed original request or credential")
	}
	if size > 4<<20 {
		if result.ExitCode != 2 || result.Client.Status != 502 || result.Client.Body != "provider response could not be processed" ||
			result.Client.Headers.Get("Content-Type") != "text/plain; charset=utf-8" {
			return errors.New("Gemini oversized body was not rejected with exact terminal 502")
		}
		return nil
	}
	if result.ExitCode != 0 || result.Client.Status != 200 || result.Client.Headers.Get("Content-Type") != "application/json; charset=utf-8" {
		return errors.New("Gemini admitted body status, type or exit mismatch")
	}
	var root map[string]any
	if json.Unmarshal([]byte(result.Client.Body), &root) != nil || geminiBufferedIdentityInvariant043711(root) != nil {
		return errors.New("Gemini admitted body lost its typed identity")
	}
	projected, err := geminiBufferedWireProjection04384(result.Client.Body)
	if err != nil || projected != geminiBufferedBodySuccess04384 {
		return errors.New("Gemini admitted body bytes differ after validated UUID normalization")
	}
	return nil
}

func geminiBufferedBodyNegativeControl04384() bool {
	for _, scenario := range []string{"buffered-body-under", "buffered-body-limit", "buffered-body-over", "buffered-body-over-rotation"} {
		body := strings.Replace(geminiBufferedBodySuccess04384, "response-uuid", "resp_deepseek_01900000-0000-7000-8000-000000000001", 1)
		run := geminiCandidateResult{Client: exchange{Status: 200, Body: body, Headers: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}},
			Upstream: []geminiCapturedRequest{{Method: "POST", Path: "/v1beta/openai/chat/completions", Auth: true, Slot: "single",
				Body: map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
					"messages": []any{map[string]any{"role": "user", "content": "same request"}}}}}}
		if size, _ := geminiBufferedBodyCase04384(scenario); size > 4<<20 {
			run.ExitCode, run.Client.Status, run.Client.Body = 2, 502, "provider response could not be processed"
			run.Client.Headers.Set("Content-Type", "text/plain; charset=utf-8")
		}
		if scenario == "buffered-body-over-rotation" {
			run.Upstream[0].Slot = "primary"
		}
		run.Stdout = run.Client.Body + "\n"
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "text/plain") },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.Upstream = nil },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream[0].Slot = "secondary" },
			func(r *geminiCandidateResult) { r.Upstream[0].Body = map[string]any{} },
		} {
			changed := run
			changed.Client.Headers = run.Client.Headers.Clone()
			changed.Upstream = append([]geminiCapturedRequest(nil), run.Upstream...)
			mutate(&changed)
			if len(geminiCompare(scenario, changed, changed)) == 0 {
				return false
			}
		}
	}
	return true
}
