package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

const geminiReasoningChoiceReason04384 = "Gemini OpenAI-compatible thinking mode currently rejects explicit tool_choice on the OpenAI Chat route, so Prodex omits it while preserving translated function tools"

func geminiReasoningChoiceSuffixes04384() []string {
	return []string{"reasoning-choice-auto", "reasoning-choice-null", "reasoning-choice-false", "reasoning-choice-object", "reasoning-choice-minimal", "reasoning-choice-stream", "reasoning-choice-stream-minimal", "reasoning-choice-stream-control", "reasoning-choice-fallback"}
}

func geminiReasoningChoiceFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	effort, choice, stream := "high", `"auto"`, false
	switch suffix {
	case "reasoning-choice-auto", "reasoning-choice-fallback":
	case "reasoning-choice-null":
		choice = "null"
	case "reasoning-choice-false":
		choice = "false"
	case "reasoning-choice-object":
		choice = `{"synthetic":"<&","value":1.0}`
	case "reasoning-choice-minimal":
		effort = "minimal"
	case "reasoning-choice-stream":
		stream = true
	case "reasoning-choice-stream-minimal":
		effort, stream = "minimal", true
	case "reasoning-choice-stream-control":
		effort, stream = "", true
	default:
		return requestPrimitiveFixture04384{}, false
	}
	streamValue := "false"
	if stream {
		streamValue = "true"
	}
	reasoning, wireReasoning := "", ""
	if effort != "" {
		reasoning, wireReasoning = `,"reasoning":{"effort":"`+effort+`"}`, `,"reasoning_effort":"`+effort+`"`
	}
	fixture := requestPrimitiveFixture04384{
		body:               `{"model":"gemini-3.1-pro-preview","input":"same request","stream":` + streamValue + reasoning + `,"tool_choice":` + choice + `}`,
		wireBody:           `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview"` + wireReasoning + `,"stream":` + streamValue,
		geminiChatStream:   stream,
		geminiChatFallback: suffix == "reasoning-choice-fallback",
	}
	if stream {
		fixture.wireBody += `,"stream_options":{"include_usage":true}`
	}
	if effort == "minimal" || effort == "" {
		fixture.wireBody += `,"tool_choice":` + choice
	} else {
		fixture.geminiOmittedChoice = choice
	}
	fixture.wireBody += "}"
	return fixture, true
}

func geminiReasoningChoiceNote04384(choice string) string {
	return `"omitted_tool_choice":{"from":` + choice + `,"reason":"` + geminiReasoningChoiceReason04384 + `"}`
}

func geminiReasoningChoiceStreamFixture04384(choice string) string {
	body := geminiChatSSEFixture043711("resp_deepseek_synthetic", "msg_gemini_synthetic", 1)
	if choice != "" {
		body = strings.Replace(body, `"finish_reason":"stop"`, `"finish_reason":"stop",`+geminiReasoningChoiceNote04384(choice), 1)
	}
	return body
}

func geminiReasoningChoiceStreamInvariant04384(scenario string, result geminiCandidateResult) error {
	fixture, _ := requestPrimitiveCase04384(scenario)
	if result.ClientError != "" || len(result.StateIntegrity) != 0 || result.ExitCode != 0 || result.Client.Status != 200 ||
		result.Client.Headers.Get("Content-Type") != "text/event-stream; charset=utf-8" || result.Stdout != result.Client.Body+"\n" || len(result.Upstream) != 1 {
		return errors.New("Gemini reasoning stream lost status, type, stdout, attempt or durable state")
	}
	wire := fixture.wireBody
	var expected map[string]any
	if json.Unmarshal([]byte(wire), &expected) != nil {
		return errors.New("invalid reasoning stream wire fixture")
	}
	attempt := result.Upstream[0]
	if attempt.RawBody != wire || !reflect.DeepEqual(attempt.Body, expected) || !attempt.Auth || attempt.Slot != "single" ||
		attempt.Method != "POST" || attempt.Path != "/v1beta/openai/chat/completions" {
		return errors.New("Gemini reasoning stream changed original request, route or credential")
	}
	body := result.Client.Body
	if fixture.geminiOmittedChoice != "" {
		note := "," + geminiReasoningChoiceNote04384(fixture.geminiOmittedChoice)
		metadata := `"metadata":{"gemini":{"finish_reason":"stop"` + note + `}}`
		if strings.Count(body, note) != 1 || strings.Count(body, metadata) != 1 {
			return errors.New("Gemini reasoning stream lost exact omission metadata")
		}
		body = strings.Replace(body, note, "", 1)
	}
	_, err := geminiChatSSEProjection043711(body)
	return err
}

func geminiReasoningChoiceNegativeControl04384() bool {
	for _, suffix := range geminiReasoningChoiceSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		for _, mutate := range []func(string) string{
			func(s string) string { return strings.ReplaceAll(s, geminiReasoningChoiceReason04384, "wrong reason") },
			func(s string) string { return strings.ReplaceAll(s, "omitted_tool_choice", "wrong_field") },
			func(s string) string {
				return strings.ReplaceAll(s, `"from":`+fixture.geminiOmittedChoice, `"from":"wrong"`)
			},
			func(s string) string {
				note := "," + geminiReasoningChoiceNote04384(fixture.geminiOmittedChoice)
				s = strings.Replace(s, note, "", 1)
				return strings.Replace(s, `"response":{"id":"resp_deepseek_synthetic"}`, `"response":{"id":"resp_deepseek_synthetic"`+note+`}`, 1)
			},
		} {
			run := geminiPrimitiveExpected04384(scenario)
			body := mutate(run.Client.Body)
			if body == run.Client.Body {
				continue
			}
			run.Client.Body, run.Stdout = body, body+"\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
