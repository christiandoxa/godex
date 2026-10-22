package main

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
)

const (
	geminiBoundResponseCase04384   = "raw-key-bound-response-same-process"
	geminiUnboundResponseCase04384 = "raw-key-unbound-response"
)

func geminiContinuationCase04384(scenario string) bool {
	return scenario == geminiBoundResponseCase04384 || scenario == geminiUnboundResponseCase04384 || geminiRestartCase04384(scenario)
}

func geminiContinuationResponse04384() string {
	body, _ := geminiSourceIDResponse04384("buffered-source-id-string")
	return strings.Replace(body, "native-preserved", "chatcmpl-differential", 1)
}

func geminiContinuationOwners04384(scenario string, result geminiCandidateResult, endpoint string) map[string]fixtureContinuationOwner {
	if result.Seed != nil {
		result = *result.Seed
	}
	if result.Client.Seed != nil {
		result.Client = *result.Client.Seed
	}
	if len(result.Upstream) == 0 || !result.Upstream[len(result.Upstream)-1].Auth {
		return nil
	}
	var upstream []upstreamRequest
	for _, request := range result.Upstream {
		upstream = append(upstream, upstreamRequest{KeySlot: request.Slot})
	}
	owners := fixtureContinuationOwners(result.Client, upstream, endpoint, nil)
	for id, owner := range owners {
		owner.provider = "gemini"
		owners[id] = owner
	}
	if scenario == geminiUnboundResponseCase04384 {
		if owner, verified := owners["chatcmpl-differential"]; verified {
			owners["chatcmpl-unknown"] = owner
		}
	}
	if session, reject, known := geminiRawKeyIdentity04384(scenario); known && !reject {
		for _, owner := range owners {
			owner.kind = "session"
			owners[session] = owner
			break
		}
	}
	if session := geminiCompactOwnershipSession04384(scenario); session != "" {
		for _, owner := range owners {
			owner.kind = "session"
			owners[session] = owner
			break
		}
	}
	return owners
}

func geminiContinuationInvariant04384(scenario string, result geminiCandidateResult) error {
	if geminiRestartCase04384(scenario) {
		if result.Seed == nil || !result.Seed.ContinuationPersisted || geminiContinuationInvariant04384(geminiUnboundResponseCase04384, *result.Seed) != nil {
			return errors.New("Gemini restart lacks an exact successful seed with durable ownership")
		}
	}
	if result.ClientError != "" || len(result.Upstream) != 1 || result.Stdout != result.Client.Body+"\n" || len(result.StateIntegrity) != 0 {
		return errors.New("Gemini raw-key lineage lost client response, one original attempt or durable state")
	}
	request := result.Upstream[0]
	want := map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
		"messages": []any{map[string]any{"role": "user", "content": "same request"}}}
	if request.Method != "POST" || request.Path != "/v1beta/openai/chat/completions" || !request.Auth || request.Slot != "single" || !reflect.DeepEqual(request.Body, want) {
		return errors.New("Gemini raw-key lineage changed original request or credential")
	}
	body := strings.Replace(geminiBufferedBodySuccess04384, "response-uuid", "chatcmpl-differential", 1)
	if scenario == geminiBoundResponseCase04384 {
		seed := result.Client.Seed
		if seed == nil || seed.Seed != nil || seed.Status != 200 || seed.Body != body || seed.Headers.Get("Content-Type") != "application/json; charset=utf-8" {
			return errors.New("Gemini same-process continuation lost its exact successful seed")
		}
	}
	status, exit, contentType := 200, 0, "application/json; charset=utf-8"
	if scenario != geminiUnboundResponseCase04384 {
		body = `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}`
		status, exit, contentType = 503, 2, "application/json"
	}
	if result.Client.Body != body || result.Client.Status != status || result.ExitCode != exit || result.Client.Headers.Get("Content-Type") != contentType {
		return errors.New("Gemini raw-key bound rejection or unbound fresh response differs exactly")
	}
	return nil
}

func geminiContinuationNegativeControl04384() bool {
	for _, scenario := range []string{geminiBoundResponseCase04384, geminiUnboundResponseCase04384,
		geminiRestartResponseCase04384, geminiRestartKeyCase04384, geminiRestartEndpointCase04384} {
		body := strings.Replace(geminiBufferedBodySuccess04384, "response-uuid", "chatcmpl-differential", 1)
		run := geminiCandidateResult{Client: exchange{Status: 200, Body: body, Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}},
			Upstream: []geminiCapturedRequest{{Method: "POST", Path: "/v1beta/openai/chat/completions", Auth: true, Slot: "single",
				Body: map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
					"messages": []any{map[string]any{"role": "user", "content": "same request"}}}}}}
		if scenario == geminiBoundResponseCase04384 {
			seed := run.Client
			seed.Headers = run.Client.Headers.Clone()
			run.Client.Seed = &seed
		}
		if geminiRestartCase04384(scenario) {
			seed := run
			seed.Client.Headers = run.Client.Headers.Clone()
			seed.Stdout, seed.ContinuationPersisted = seed.Client.Body+"\n", true
			run.Seed = &seed
		}
		if scenario != geminiUnboundResponseCase04384 {
			run.Client.Body = `{"error":{"code":"bound_continuation_unavailable","message":"bound continuation provider is unavailable"}}`
			run.Client.Status, run.ExitCode = 503, 2
			run.Client.Headers.Set("Content-Type", "application/json")
		}
		run.Stdout = run.Client.Body + "\n"
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		mutations := []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.ClientError = "missing exchange" },
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "text/plain") },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.Upstream = nil },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream[0].Slot = "secondary" },
			func(r *geminiCandidateResult) { r.Upstream[0].Body = map[string]any{} },
		}
		if geminiRestartCase04384(scenario) {
			mutations = append(mutations,
				func(r *geminiCandidateResult) { r.Seed = nil },
				func(r *geminiCandidateResult) { r.Seed.ContinuationPersisted = false },
				func(r *geminiCandidateResult) { r.Seed.ExitCode = 2 },
				func(r *geminiCandidateResult) { r.Seed.Client.Status = 503 },
				func(r *geminiCandidateResult) { r.Seed.Client.Body = `{}` },
				func(r *geminiCandidateResult) { r.Seed.ClientError = "missing seed exchange" },
				func(r *geminiCandidateResult) { r.Seed.Upstream = nil },
				func(r *geminiCandidateResult) { r.Seed.Stdout = "changed" },
				func(r *geminiCandidateResult) { r.Seed.StateIntegrity = []string{"changed"} })
		}
		if scenario == geminiBoundResponseCase04384 {
			mutations = append(mutations,
				func(r *geminiCandidateResult) { r.Client.Seed = nil },
				func(r *geminiCandidateResult) { r.Client.Seed.Status = 503 },
				func(r *geminiCandidateResult) { r.Client.Seed.Body = `{}` },
				func(r *geminiCandidateResult) { r.Client.Seed.Headers.Set("Content-Type", "text/plain") })
		}
		for _, mutate := range mutations {
			changed := run
			changed.Client.Headers = run.Client.Headers.Clone()
			if run.Client.Seed != nil {
				seed := *run.Client.Seed
				seed.Headers = run.Client.Seed.Headers.Clone()
				changed.Client.Seed = &seed
			}
			changed.Upstream = append([]geminiCapturedRequest(nil), run.Upstream...)
			if run.Seed != nil {
				seed := *run.Seed
				changed.Seed = &seed
			}
			mutate(&changed)
			if len(geminiCompare(scenario, changed, changed)) == 0 {
				return false
			}
		}
	}
	return true
}
