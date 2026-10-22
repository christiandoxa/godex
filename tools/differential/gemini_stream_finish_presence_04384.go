package main

import (
	"errors"
	"slices"
	"strings"
)

func geminiStreamFinishPresenceCases04384() []string {
	return []string{"stream-chat-finish-empty", "stream-chat-finish-empty-initial", "stream-chat-finish-null-control", "stream-chat-finish-wrong-control", "stream-chat-finish-absent-control"}
}

func geminiStreamFinishPresenceCase04384(scenario string) bool {
	return slices.Contains(geminiStreamFinishPresenceCases04384(), scenario)
}

func geminiStreamFinishPresenceFixture04384(scenario string) (first, second, last, expected string) {
	first, second, last, expected = `"stop"`, `null`, `""`, `""`
	switch scenario {
	case "stream-chat-finish-empty-initial":
		first, last = `""`, ""
	case "stream-chat-finish-null-control":
		last, expected = `null`, first
	case "stream-chat-finish-wrong-control":
		second, last, expected = `false`, `{}`, first
	case "stream-chat-finish-absent-control":
		first, second, last, expected = "", "", "", ""
	}
	return first, second, last, expected
}

func geminiStreamFinishPresencePacket04384(packet, finish string) string {
	field := ""
	if finish != "" {
		field = `,"finish_reason":` + finish
	}
	return strings.NewReplacer(`,"finish_reason":null`, field, `,"finish_reason":"stop"`, field).Replace(packet)
}

func geminiStreamFinishPresenceExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	_, _, _, finish := geminiStreamFinishPresenceFixture04384(scenario)
	if finish == "" {
		run.Client.Body = strings.Replace(run.Client.Body, `"metadata":{"gemini":{"finish_reason":"stop"}},`, "", 1)
	} else {
		run.Client.Body = strings.Replace(run.Client.Body, `"finish_reason":"stop"`, `"finish_reason":`+finish, 1)
	}
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiStreamFinishPresenceInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini stream finish reason lost exact stdout")
	}
	_, _, _, finish := geminiStreamFinishPresenceFixture04384(scenario)
	if finish == "" {
		if strings.Contains(run.Client.Body, `"metadata":`) {
			return errors.New("Gemini stream invented an absent finish reason")
		}
		run.Client.Body = strings.Replace(run.Client.Body, `"model":`, `"metadata":{"gemini":{"finish_reason":"stop"}},"model":`, 1)
	} else if finish == `""` {
		if strings.Count(run.Client.Body, `"finish_reason":""`) != 1 {
			return errors.New("Gemini stream dropped an explicitly empty finish reason")
		}
		run.Client.Body = strings.Replace(run.Client.Body, `"finish_reason":""`, `"finish_reason":"stop"`, 1)
	}
	run.Stdout = run.Client.Body + "\n"
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamFinishPresenceNegativeControl04384() bool {
	for _, scenario := range geminiStreamFinishPresenceCases04384() {
		good := geminiStreamFinishPresenceExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"model":`, `"metadata":{"gemini":{"finish_reason":"must-not-pass"}},"model":`, 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamFinishPresenceExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
