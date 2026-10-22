package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type requestPrimitiveFixture04384 struct {
	fields, limit string
	reject        bool
}

func requestPrimitiveSuffixes04384() []string {
	return []string{"output", "precedence", "all", "shadow-null", "shadow-zero", "shadow-fraction", "u64", "overflow", "parallel-true"}
}

func requestPrimitiveCase04384(scenario string) (requestPrimitiveFixture04384, bool) {
	if !strings.HasPrefix(scenario, "request-primitive-") && !strings.HasPrefix(scenario, "deepseek-request-primitive-") {
		return requestPrimitiveFixture04384{}, false
	}
	suffix := strings.TrimPrefix(strings.TrimPrefix(scenario, "deepseek-"), "request-primitive-")
	switch suffix {
	case "output":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":11`, limit: "11"}, true
	case "precedence":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":11,"max_tokens":23`, limit: "23"}, true
	case "all":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":11,"max_tokens":23,"max_completion_tokens":31`, limit: "31"}, true
	case "shadow-null":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":null,"max_completion_tokens":31`, reject: true}, true
	case "shadow-zero":
		return requestPrimitiveFixture04384{fields: `"max_tokens":0,"max_completion_tokens":31`, reject: true}, true
	case "shadow-fraction":
		return requestPrimitiveFixture04384{fields: `"max_output_tokens":1.0,"max_completion_tokens":31`, reject: true}, true
	case "u64":
		return requestPrimitiveFixture04384{fields: `"max_tokens":18446744073709551615`, limit: "18446744073709551615"}, true
	case "overflow":
		return requestPrimitiveFixture04384{fields: `"max_tokens":18446744073709551616`, reject: true}, true
	case "parallel-true":
		return requestPrimitiveFixture04384{fields: `"parallel_tool_calls":true`}, true
	}
	return requestPrimitiveFixture04384{}, false
}

func requestPrimitiveInput04384(scenario, model string) []byte {
	fixture, _ := requestPrimitiveCase04384(scenario)
	return []byte(`{"model":"` + model + `","input":"same request",` + fixture.fields + `}`)
}

func requestPrimitiveWire04384(scenario, model string) string {
	fixture, _ := requestPrimitiveCase04384(scenario)
	body := "{"
	if fixture.limit != "" {
		body += `"max_tokens":` + fixture.limit + ","
	}
	return body + `"messages":[{"content":"same request","role":"user"}],"model":"` + model + `","stream":false}`
}

func geminiPrimitiveExpected04384(scenario string) geminiCandidateResult {
	fixture, _ := requestPrimitiveCase04384(scenario)
	if fixture.reject {
		return geminiUserIDExpected04384("request-user-id-number")
	}
	result := geminiUserIDExpected04384("request-user-id-absent")
	result.Upstream[0].RawBody = requestPrimitiveWire04384(scenario, "gemini-3.1-pro-preview")
	_ = json.Unmarshal([]byte(result.Upstream[0].RawBody), &result.Upstream[0].Body)
	return result
}

func deepSeekPrimitiveExpected04384(scenario string) productRun {
	fixture, _ := requestPrimitiveCase04384(scenario)
	result := productRun{Client: exchange{Status: 200, Headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body: `{"created_at":1,"id":"chatcmpl-differential","metadata":{"deepseek":{"finish_reason":"stop"}},"model":"deepseek-v4-pro","object":"response","output":[{"content":[{"text":"synthetic-ok","type":"output_text"}],"role":"assistant","type":"message"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`}}
	if fixture.reject {
		result.ExitStatus = 2
		result.Client = exchange{Status: 502, Headers: http.Header{"Content-Type": {"text/plain; charset=utf-8"}}, Body: "upstream request failed"}
	} else {
		result.Upstream = []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: requestPrimitiveWire04384(scenario, "deepseek-v4-pro")}}
	}
	result.Stdout = result.Client.Body + "\n"
	return result
}

func deepSeekPrimitiveInvariants04384(scenario scenarioResult) []string {
	if len(scenario.Runs) != 2 {
		return []string{scenario.Name + ".run_count"}
	}
	want := deepSeekPrimitiveExpected04384(scenario.Name)
	var failures []string
	for _, run := range scenario.Runs {
		if run.ClientError != "" || len(run.StateIntegrity) != 0 || run.Retries != 0 || run.Cancelled || run.ExitStatus != want.ExitStatus ||
			run.Client.Status != want.Client.Status || run.Client.Body != want.Client.Body || run.Client.Headers.Get("Content-Type") != want.Client.Headers.Get("Content-Type") ||
			run.Stdout != want.Stdout || len(run.Upstream) != len(want.Upstream) {
			failures = append(failures, scenario.Name+"."+run.Name+".exact_admission_or_response")
			continue
		}
		for index, request := range run.Upstream {
			expected := want.Upstream[index]
			if request.Method != expected.Method || request.Path != expected.Path || !request.AuthOK || request.KeySlot != expected.KeySlot || request.Body != expected.Body {
				failures = append(failures, scenario.Name+"."+run.Name+".exact_original_request")
			}
		}
	}
	return failures
}

func geminiPrimitiveInvariant04384(scenario string, result geminiCandidateResult) error {
	if !geminiExactRequestResult04384(result, geminiPrimitiveExpected04384(scenario)) {
		return errors.New("Gemini primitive controls lost exact limit, admission, request or response")
	}
	return nil
}

func requestPrimitiveNegativeControl04384() bool {
	for _, suffix := range requestPrimitiveSuffixes04384() {
		scenario := "request-primitive-" + suffix
		gemini := geminiPrimitiveExpected04384(scenario)
		deepseek := deepSeekPrimitiveExpected04384(scenario)
		if len(geminiCompare(scenario, gemini, gemini)) != 0 || len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: "deepseek-" + scenario, Runs: []productRun{deepseek, deepseek}})) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.Body = `{}` },
			func(r *geminiCandidateResult) { r.Client.Headers.Set("Content-Type", "application/json") },
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Stdout = "changed" },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"changed"} },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, geminiCapturedRequest{}) },
		} {
			changed := gemini
			changed.Client.Headers = gemini.Client.Headers.Clone()
			changed.Upstream = append([]geminiCapturedRequest(nil), gemini.Upstream...)
			mutate(&changed)
			if len(geminiCompare(scenario, changed, changed)) == 0 {
				return false
			}
		}
		for _, mutate := range []func(*productRun){
			func(r *productRun) { r.Client.Status++ },
			func(r *productRun) { r.Client.Body = `{}` },
			func(r *productRun) { r.Client.Headers.Set("Content-Type", "application/json") },
			func(r *productRun) { r.ExitStatus++ },
			func(r *productRun) { r.Stdout = "changed" },
			func(r *productRun) { r.StateIntegrity = []string{"changed"} },
			func(r *productRun) { r.ClientError = "missing exchange" },
			func(r *productRun) { r.Upstream = append(r.Upstream, upstreamRequest{}) },
			func(r *productRun) { r.Retries++ },
		} {
			changed := deepseek
			changed.Client.Headers = deepseek.Client.Headers.Clone()
			changed.Upstream = append([]upstreamRequest(nil), deepseek.Upstream...)
			mutate(&changed)
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: "deepseek-" + scenario, Runs: []productRun{changed, changed}})) == 0 {
				return false
			}
		}
		if len(gemini.Upstream) != 0 {
			for _, field := range []string{"wire", "auth", "slot"} {
				changedGemini, changedDeepSeek := geminiPrimitiveExpected04384(scenario), deepSeekPrimitiveExpected04384(scenario)
				switch field {
				case "wire":
					changedGemini.Upstream[0].RawBody, changedDeepSeek.Upstream[0].Body = `{}`, `{}`
				case "auth":
					changedGemini.Upstream[0].Auth, changedDeepSeek.Upstream[0].AuthOK = false, false
				case "slot":
					changedGemini.Upstream[0].Slot, changedDeepSeek.Upstream[0].KeySlot = "secondary", "secondary"
				}
				if len(geminiCompare(scenario, changedGemini, changedGemini)) == 0 ||
					len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: "deepseek-" + scenario, Runs: []productRun{changedDeepSeek, changedDeepSeek}})) == 0 {
					return false
				}
			}
		}
	}
	return true
}
