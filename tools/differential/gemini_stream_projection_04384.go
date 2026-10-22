package main

import (
	"errors"
	"slices"
)

const geminiProjectionReadError04384 = "context deadline exceeded (Client.Timeout or context cancellation while reading body)"

func geminiStreamProjectionCases04384() []string {
	return []string{"stream-chat-tool-entry-null", "stream-chat-tool-entry-bool", "stream-chat-tool-entry-number", "stream-chat-tool-entry-string", "stream-chat-tool-entry-array", "stream-chat-tool-calls-empty-control", "stream-chat-tool-calls-null-control", "stream-chat-tool-calls-object-control", "stream-chat-tool-calls-bool-control", "stream-chat-tool-calls-absent-control"}
}

func geminiStreamProjectionCase04384(scenario string) bool {
	return slices.Contains(geminiStreamProjectionCases04384(), scenario)
}

func geminiStreamProjectionFixture04384(scenario string) (calls string, pending bool) {
	switch scenario {
	case "stream-chat-tool-entry-null":
		return `[null]`, true
	case "stream-chat-tool-entry-bool":
		return `[false]`, true
	case "stream-chat-tool-entry-number":
		return `[0]`, true
	case "stream-chat-tool-entry-string":
		return `[""]`, true
	case "stream-chat-tool-entry-array":
		return `[[]]`, true
	case "stream-chat-tool-calls-empty-control":
		return `[]`, false
	case "stream-chat-tool-calls-null-control":
		return `null`, false
	case "stream-chat-tool-calls-object-control":
		return `{}`, false
	case "stream-chat-tool-calls-bool-control":
		return `false`, false
	}
	return "", false
}

func geminiStreamProjectionExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	if _, pending := geminiStreamProjectionFixture04384(scenario); pending {
		run.ExitCode, run.Client.Body, run.Client.ReadError, run.Stdout = 1, "", geminiProjectionReadError04384, "\n"
	}
	return run
}

func geminiStreamProjectionInvariant04384(scenario string, run geminiCandidateResult) error {
	if _, pending := geminiStreamProjectionFixture04384(scenario); pending {
		if run.ExitCode != 1 || run.Client.Body != "" || run.Stdout != "\n" || run.Client.ReadError != geminiProjectionReadError04384 {
			return errors.New("Gemini nonobject tool projection changed pending response or cancellation")
		}
		// Verify the failed-read tuple before reusing the independent request/state guard.
		baseline := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
		run.ExitCode, run.Client.Body, run.Client.ReadError, run.Stdout = 0, baseline.Client.Body, "", baseline.Stdout
	} else if run.Client.ReadError != "" {
		return errors.New("Gemini completed projection control retained a read error")
	}
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamProjectionNegativeControl04384() bool {
	for _, scenario := range geminiStreamProjectionCases04384() {
		good := geminiStreamProjectionExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.ExitCode = 99 },
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.ReadError = "wrong failure" },
			func(r *geminiCandidateResult) { r.Client.Body += "forged" },
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamProjectionExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
