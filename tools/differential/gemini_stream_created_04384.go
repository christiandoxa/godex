package main

import (
	"errors"
	"slices"
	"strings"
)

func geminiStreamCreatedCases04384() []string {
	return []string{"stream-chat-created", "stream-chat-created-zero", "stream-chat-created-large", "stream-chat-created-u64", "stream-chat-created-update", "stream-chat-created-float-control", "stream-chat-created-overflow-control"}
}

func geminiStreamCreatedCase04384(scenario string) bool {
	return slices.Contains(geminiStreamCreatedCases04384(), scenario)
}

func geminiStreamCreatedFixture04384(scenario string) (first, second, early, late string) {
	first = "123"
	switch scenario {
	case "stream-chat-created-zero":
		first = "0"
	case "stream-chat-created-large":
		first = "9007199254740993"
	case "stream-chat-created-u64":
		first = "18446744073709551615"
	case "stream-chat-created-update":
		return "111", "222", "111", "222"
	case "stream-chat-created-float-control":
		return "1.0", "-1", "", ""
	case "stream-chat-created-overflow-control":
		return "18446744073709551616", "null", "", ""
	}
	return first, "", first, first
}

func geminiStreamCreatedPacket04384(packet, created string) string {
	if created == "" {
		return packet
	}
	return strings.Replace(packet, `"id":`, `"created":`+created+`,"id":`, 1)
}

func geminiStreamCreatedExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	_, _, early, late := geminiStreamCreatedFixture04384(scenario)
	if early != "" {
		run.Client.Body = strings.ReplaceAll(run.Client.Body, `"created_at":111,`, `"created_at":`+late+`,`)
		run.Client.Body = strings.Replace(run.Client.Body, `"created_at":`+late+`,`, `"created_at":`+early+`,`, 2)
		run.Stdout = run.Client.Body + "\n"
	}
	return run
}

func geminiStreamCreatedInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini stream timestamp lost exact stdout")
	}
	_, _, early, late := geminiStreamCreatedFixture04384(scenario)
	if early != "" {
		first, last := `"created_at":`+early+`,`, `"created_at":`+late+`,`
		remaining := run.Client.Body
		for _, expected := range []string{first, first, last, last} {
			index := strings.Index(remaining, `"created_at":`)
			if index < 0 || !strings.HasPrefix(remaining[index:], expected) {
				return errors.New("Gemini stream lost exact unsigned upstream timestamp order")
			}
			remaining = remaining[index+len(expected):]
		}
		if strings.Contains(remaining, `"created_at":`) {
			return errors.New("Gemini stream added an unexpected timestamp")
		}
		run.Client.Body = strings.ReplaceAll(run.Client.Body, first, `"created_at":111,`)
		run.Client.Body = strings.ReplaceAll(run.Client.Body, last, `"created_at":111,`)
		run.Stdout = run.Client.Body + "\n"
	}
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamCreatedNegativeControl04384() bool {
	for _, scenario := range geminiStreamCreatedCases04384() {
		good := geminiStreamCreatedExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"created_at":`, `"wrong":`, 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamCreatedExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
