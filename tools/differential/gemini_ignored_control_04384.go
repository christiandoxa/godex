package main

import (
	"errors"
	"strings"
)

func geminiIgnoredControlCases04384() []string {
	var cases []string
	for _, field := range []string{"parallel", "presence_penalty", "frequency_penalty", "seed", "n"} {
		prefix := "request-ignored-" + field + "-"
		if field == "parallel" {
			prefix = "request-parallel-"
		}
		for _, suffix := range []string{"true", "false", "null", "number", "string", "array", "object"} {
			cases = append(cases, prefix+suffix)
		}
	}
	return cases
}

func geminiIgnoredControl04384(scenario string) (string, string, bool) {
	field, suffix, known := strings.Cut(strings.TrimPrefix(scenario, "request-ignored-"), "-")
	if strings.HasPrefix(scenario, "request-parallel-") {
		field, suffix, known = "parallel_tool_calls", strings.TrimPrefix(scenario, "request-parallel-"), true
	} else if !strings.HasPrefix(scenario, "request-ignored-") {
		return "", "", false
	}
	if !known || field != "parallel_tool_calls" && field != "presence_penalty" && field != "frequency_penalty" && field != "seed" && field != "n" {
		return "", "", false
	}
	value := ""
	switch suffix {
	case "true", "false", "null":
		value = suffix
	case "number":
		value = "7"
	case "string":
		value = `"opaque"`
	case "array":
		value = `[]`
	case "object":
		value = `{}`
	default:
		return "", "", false
	}
	return field, value, true
}

func geminiIgnoredInput04384(scenario string) []byte {
	field, value, _ := geminiIgnoredControl04384(scenario)
	return []byte(`{"model":"gemini-3.1-pro-preview","input":"same request","` + field + `":` + value + `}`)
}

func geminiIgnoredInvariant04384(result geminiCandidateResult) error {
	if err := geminiContinuationInvariant04384(geminiUnboundResponseCase04384, result); err != nil {
		return err
	}
	const wire = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	if result.Upstream[0].RawBody != wire {
		return errors.New("ignored Gemini control changed exact original upstream bytes")
	}
	return nil
}

func geminiIgnoredNegativeControl04384() bool {
	for _, scenario := range geminiIgnoredControlCases04384() {
		run := geminiUserIDExpected04384("request-user-id-absent")
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		field, value, _ := geminiIgnoredControl04384(scenario)
		run.Upstream[0].RawBody = strings.TrimSuffix(run.Upstream[0].RawBody, "}") + `,"` + field + `":` + value + `}`
		if len(geminiCompare(scenario, run, run)) == 0 {
			return false
		}
	}
	return true
}
