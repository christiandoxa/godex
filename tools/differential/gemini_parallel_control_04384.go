package main

import (
	"errors"
	"strings"
)

func geminiParallelControl04384(scenario string) (string, bool) {
	switch scenario {
	case "request-parallel-true":
		return "true", true
	case "request-parallel-false":
		return "false", true
	case "request-parallel-null":
		return "null", true
	case "request-parallel-number":
		return "7", true
	case "request-parallel-string":
		return `"opaque"`, true
	case "request-parallel-array":
		return `[]`, true
	case "request-parallel-object":
		return `{}`, true
	}
	return "", false
}

func geminiParallelInput04384(scenario string) []byte {
	value, _ := geminiParallelControl04384(scenario)
	return []byte(`{"model":"gemini-3.1-pro-preview","input":"same request","parallel_tool_calls":` + value + `}`)
}

func geminiParallelInvariant04384(result geminiCandidateResult) error {
	if err := geminiContinuationInvariant04384(geminiUnboundResponseCase04384, result); err != nil {
		return err
	}
	const wire = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
	if result.Upstream[0].RawBody != wire {
		return errors.New("ignored Gemini parallel-tool control changed exact original upstream bytes")
	}
	return nil
}

func geminiParallelNegativeControl04384() bool {
	for _, scenario := range []string{"request-parallel-true", "request-parallel-false", "request-parallel-null", "request-parallel-number", "request-parallel-string", "request-parallel-array", "request-parallel-object"} {
		run := geminiUserIDExpected04384("request-user-id-absent")
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		value, _ := geminiParallelControl04384(scenario)
		run.Upstream[0].RawBody = strings.TrimSuffix(run.Upstream[0].RawBody, "}") + `,"parallel_tool_calls":` + value + `}`
		if len(geminiCompare(scenario, run, run)) == 0 {
			return false
		}
	}
	return true
}
