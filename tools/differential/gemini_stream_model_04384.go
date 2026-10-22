package main

import (
	"errors"
	"slices"
	"strings"
)

func geminiStreamModelCases04384() []string {
	return []string{"stream-chat-model-empty", "stream-chat-model-empty-initial", "stream-chat-model-null-control", "stream-chat-model-wrong-control", "stream-chat-model-absent-control"}
}

func geminiStreamModelCase04384(scenario string) bool {
	return slices.Contains(geminiStreamModelCases04384(), scenario)
}

func geminiStreamModelFixture04384(scenario string) (first, second, model string) {
	first, second, model = `"gemini-3.1-pro-preview"`, `""`, `""`
	switch scenario {
	case "stream-chat-model-empty-initial":
		first, second = `""`, ""
	case "stream-chat-model-null-control":
		second, model = `null`, first
	case "stream-chat-model-wrong-control":
		second, model = `{}`, first
	case "stream-chat-model-absent-control":
		first, second, model = "", "", ""
	}
	return first, second, model
}

func geminiStreamModelPacket04384(packet, model string) string {
	if model == "" {
		return strings.Replace(packet, `"model":"gemini-3.1-pro-preview",`, "", 1)
	}
	return strings.Replace(packet, `"model":"gemini-3.1-pro-preview"`, `"model":`+model, 1)
}

func geminiStreamModelExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	_, _, model := geminiStreamModelFixture04384(scenario)
	run.Client.Body = geminiStreamModelPacket04384(run.Client.Body, model)
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiStreamModelInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini stream model lost exact stdout")
	}
	_, _, model := geminiStreamModelFixture04384(scenario)
	if model == "" {
		if strings.Contains(run.Client.Body, `"model":`) {
			return errors.New("Gemini stream invented an absent upstream model")
		}
		run.Client.Body = strings.Replace(run.Client.Body, `"output":[`, `"model":"gemini-3.1-pro-preview","output":[`, 1)
	} else if model == `""` {
		if strings.Count(run.Client.Body, `"model":""`) != 1 {
			return errors.New("Gemini stream dropped an explicitly empty model")
		}
		run.Client.Body = strings.Replace(run.Client.Body, `"model":""`, `"model":"gemini-3.1-pro-preview"`, 1)
	}
	run.Stdout = run.Client.Body + "\n"
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamModelNegativeControl04384() bool {
	for _, scenario := range geminiStreamModelCases04384() {
		good := geminiStreamModelExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"output":[`, `"model":"must-not-pass","output":[`, 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamModelExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
