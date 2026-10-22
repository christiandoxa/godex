package main

import (
	"errors"
	"slices"
	"strings"
)

const geminiStreamUsageBaseline04384 = `{"input_tokens":9007199254740993,"output_tokens":1,"total_tokens":9007199254740994}`

func geminiStreamUsageCases04384() []string {
	return []string{"stream-chat-usage-empty", "stream-chat-usage-null", "stream-chat-usage-missing-total", "stream-chat-usage-saturating", "stream-chat-usage-invalid-counters", "stream-chat-usage-overflow-counters", "stream-chat-usage-details", "stream-chat-usage-zero-details", "stream-chat-usage-array", "stream-chat-usage-scalar", "stream-chat-usage-valid-control", "stream-chat-usage-null-then-valid-control", "stream-chat-usage-absent-control", "stream-chat-usage-explicit-zero-control", "stream-chat-usage-retain-control"}
}

func geminiStreamUsageCase04384(scenario string) bool {
	return slices.Contains(geminiStreamUsageCases04384(), scenario)
}

func geminiStreamUsageFixture04384(scenario string) (first, last, want string) {
	valid := `{"prompt_tokens":9007199254740993,"completion_tokens":1,"total_tokens":9007199254740994}`
	last, want = valid, geminiStreamUsageBaseline04384
	zero := `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`
	switch scenario {
	case "stream-chat-usage-empty":
		last, want = `{}`, zero
	case "stream-chat-usage-null":
		first, last, want = valid, `null`, zero
	case "stream-chat-usage-missing-total":
		last = `{"prompt_tokens":9007199254740993,"completion_tokens":1}`
	case "stream-chat-usage-saturating":
		last, want = `{"prompt_tokens":18446744073709551615,"completion_tokens":1}`, `{"input_tokens":18446744073709551615,"output_tokens":1,"total_tokens":18446744073709551615}`
	case "stream-chat-usage-invalid-counters":
		last, want = `{"prompt_tokens":-1,"completion_tokens":2.0,"total_tokens":"9","prompt_cache_hit_tokens":1e2,"completion_tokens_details":{"reasoning_tokens":false}}`, zero
	case "stream-chat-usage-overflow-counters":
		last, want = `{"prompt_tokens":18446744073709551616,"completion_tokens":1}`, `{"input_tokens":0,"output_tokens":1,"total_tokens":1}`
	case "stream-chat-usage-details":
		last = `{"prompt_tokens":18446744073709551615,"completion_tokens":9223372036854775808,"total_tokens":9007199254740993,"prompt_cache_hit_tokens":9223372036854775808,"prompt_cache_miss_tokens":18446744073709551615,"completion_tokens_details":{"reasoning_tokens":9007199254740993}}`
		want = `{"input_tokens":18446744073709551615,"input_tokens_details":{"cached_tokens":9223372036854775808},"metadata":{"gemini":{"prompt_cache_hit_tokens":9223372036854775808,"prompt_cache_miss_tokens":18446744073709551615}},"output_tokens":9223372036854775808,"output_tokens_details":{"reasoning_tokens":9007199254740993},"total_tokens":9007199254740993}`
	case "stream-chat-usage-zero-details":
		last = `{"prompt_tokens":2,"completion_tokens":1,"total_tokens":0,"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":0,"completion_tokens_details":{"reasoning_tokens":0}}`
		want = `{"input_tokens":2,"input_tokens_details":{"cached_tokens":0},"metadata":{"gemini":{"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":0}},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":0}`
	case "stream-chat-usage-array":
		last, want = `[]`, zero
	case "stream-chat-usage-scalar":
		last, want = `false`, zero
	case "stream-chat-usage-null-then-valid-control":
		first = `null`
	case "stream-chat-usage-absent-control":
		last, want = "", ""
	case "stream-chat-usage-explicit-zero-control":
		last, want = `{"prompt_tokens":2,"completion_tokens":1,"total_tokens":0}`, `{"input_tokens":2,"output_tokens":1,"total_tokens":0}`
	case "stream-chat-usage-retain-control":
		first, last = valid, ""
	}
	return first, last, want
}

func geminiStreamUsageExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	_, _, usage := geminiStreamUsageFixture04384(scenario)
	field := ""
	if usage != "" {
		field = `,"usage":` + usage
	}
	run.Client.Body = strings.Replace(run.Client.Body, `,"usage":`+geminiStreamUsageBaseline04384, field, 1)
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiStreamUsageInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini stream usage lost exact stdout")
	}
	_, _, usage := geminiStreamUsageFixture04384(scenario)
	if usage == "" {
		if strings.Contains(run.Client.Body, `"usage":`) {
			return errors.New("Gemini stream invented absent usage")
		}
		marker := `},"sequence_number":5,"type":"response.completed"`
		run.Client.Body = strings.Replace(run.Client.Body, marker, `,"usage":`+geminiStreamUsageBaseline04384+marker, 1)
	} else {
		field := `,"usage":` + usage
		if strings.Count(run.Client.Body, field) != 1 {
			return errors.New("Gemini stream changed normalized usage or detail presence")
		}
		run.Client.Body = strings.Replace(run.Client.Body, field, `,"usage":`+geminiStreamUsageBaseline04384, 1)
	}
	run.Stdout = run.Client.Body + "\n"
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamUsageNegativeControl04384() bool {
	for _, scenario := range geminiStreamUsageCases04384() {
		good := geminiStreamUsageExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"model":`, `"usage":false,"model":`, 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamUsageExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
