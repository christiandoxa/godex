package main

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
)

func geminiRawKeyIdentity04384(scenario string) (value string, reject, known bool) {
	switch scenario {
	case "raw-key-session-limit":
		return strings.Repeat("x", 1024), false, true
	case "raw-key-session-over-limit":
		return strings.Repeat("x", 1025), true, true
	case "raw-key-session-unicode-limit":
		return strings.Repeat("雪", 341) + "x", false, true
	case "raw-key-session-unicode-over":
		return strings.Repeat("雪", 342), true, true
	case "raw-key-session-control":
		return "synthetic\x00session", true, true
	}
	return "", false, false
}

func geminiRawKeyIdentityInvariant04384(scenario string, result geminiCandidateResult) error {
	_, reject, known := geminiRawKeyIdentity04384(scenario)
	if !known || strings.TrimSuffix(result.Stdout, "\n") != result.Client.Body || len(result.StateIntegrity) != 0 {
		return errors.New("Gemini identity stdout or durable state mismatch")
	}
	if reject {
		if result.ExitCode != 2 || result.Client.Status != 503 || len(result.Upstream) != 0 ||
			result.Client.Headers.Get("Content-Type") != "application/json" ||
			result.Client.Body != `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}` {
			return errors.New("Gemini invalid identity was not rejected exactly before dispatch")
		}
		return nil
	}
	if result.ExitCode != 0 || result.Client.Headers.Get("Content-Type") != "application/json; charset=utf-8" {
		return errors.New("Gemini valid identity status or type mismatch")
	}
	if err := geminiFixtureInvariant("chat-request", result); err != nil {
		return err
	}
	want := map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
		"messages": []any{map[string]any{"role": "user", "content": "same request"}}}
	if result.Upstream[0].Slot != "single" || !reflect.DeepEqual(result.Upstream[0].Body, want) {
		return errors.New("Gemini valid identity changed the exact request or credential")
	}
	return nil
}

func geminiRawKeyIdentityNegativeControl04384() bool {
	for _, scenario := range []string{"raw-key-session-limit", "raw-key-session-over-limit", "raw-key-session-unicode-limit", "raw-key-session-unicode-over", "raw-key-session-control"} {
		_, reject, _ := geminiRawKeyIdentity04384(scenario)
		run := geminiCandidateResult{Client: exchange{Status: 200,
			Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}},
			Body:    `{"id":"resp_deepseek_00000000-0000-7000-8000-000000000000","model":"gemini-3.1-pro-preview","object":"response","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"synthetic-ok"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`},
			Upstream: []geminiCapturedRequest{{Method: "POST", Path: "/v1beta/openai/chat/completions", Auth: true, Slot: "single",
				Body: map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
					"messages": []any{map[string]any{"role": "user", "content": "same request"}}}}}}
		if reject {
			run.ExitCode, run.Client.Status, run.Upstream = 2, 503, nil
			run.Client.Headers.Set("Content-Type", "application/json")
			run.Client.Body = `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}`
		}
		run.Stdout = run.Client.Body + "\n"
		if differences := geminiCompare(scenario, run, run); len(differences) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "text/plain") },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, geminiCapturedRequest{}) },
		} {
			changed := run
			changed.Client.Headers = run.Client.Headers.Clone()
			mutate(&changed)
			if differences := geminiCompare(scenario, changed, changed); len(differences) == 0 {
				return false
			}
		}
	}
	if _, _, known := geminiRawKeyIdentity04384("chat-request"); known {
		return false
	}
	return true
}
