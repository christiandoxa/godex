package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func geminiStreamToolAdmissionCases04384() []string {
	return []string{"stream-chat-tool-missing-function", "stream-chat-tool-null-function", "stream-chat-tool-wrong-function", "stream-chat-tool-only-missing-function", "stream-chat-error-before-text", "stream-chat-error-after-text"}
}

func geminiStreamToolAdmissionCase04384(scenario string) bool {
	return slices.Contains(geminiStreamToolAdmissionCases04384(), scenario)
}

func geminiStreamToolAdmissionFirst04384(scenario, packet string) string {
	calls := ""
	switch scenario {
	case "stream-chat-tool-missing-function", "stream-chat-tool-only-missing-function":
		calls = `[{"index":0,"id":"call_synthetic"}]`
	case "stream-chat-tool-null-function":
		calls = `[{"index":8,"function":null}]`
	case "stream-chat-tool-wrong-function":
		calls = `[{"function":false}]`
	case "stream-chat-error-before-text":
		return `{"error":{"code":"synthetic_error","message":"synthetic semantic error"},"choices":[{"delta":{"content":"must-not-emit"}}]}`
	}
	if scenario == "stream-chat-tool-only-missing-function" {
		packet = strings.Replace(packet, `"content":"synthetic-"`, `"content":null`, 1)
	}
	if calls != "" {
		packet = strings.Replace(packet, `"delta":{`, `"delta":{"tool_calls":`+calls+",", 1)
	}
	return packet
}

func geminiStreamToolAdmissionWire04384(scenario, responseID, itemID string, epoch int64) string {
	parts := strings.Split(geminiChatSSEFixture043711(responseID, itemID, epoch), "\r\n\r\n")
	code, message := "invalid_tool_call_arguments", "Gemini OpenAI-compatible streamed a tool call without a function object"
	if strings.HasPrefix(scenario, "stream-chat-error-") {
		code, message = "synthetic_error", "synthetic semantic error"
	}
	sequence := 3
	if scenario == "stream-chat-tool-only-missing-function" {
		sequence = 1
	}
	if scenario == "stream-chat-error-before-text" {
		sequence = 0
	}
	failed := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":%q,\"message\":%q},\"id\":%q},\"sequence_number\":%d,\"type\":\"response.failed\"}", epoch, code, message, responseID, sequence)
	var events []string
	switch scenario {
	case "stream-chat-error-before-text":
		events = []string{failed, strings.Replace(parts[0], `"sequence_number":0`, `"sequence_number":1`, 1), strings.Replace(parts[1], `"sequence_number":1`, `"sequence_number":2`, 1), parts[3]}
	case "stream-chat-tool-only-missing-function":
		events = []string{parts[0], failed, strings.Replace(parts[1], `"sequence_number":1`, `"sequence_number":2`, 1), parts[3]}
	default:
		events = []string{parts[0], parts[1], parts[2], failed, strings.Replace(parts[3], `"sequence_number":3`, `"sequence_number":4`, 1)}
	}
	return strings.Join(events, "\r\n\r\n") + "\r\n\r\n"
}

func geminiStreamToolAdmissionExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	run.Client.Body = geminiStreamToolAdmissionWire04384(scenario, "resp_deepseek_fixture", "msg_gemini_fixture", 111)
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiStreamToolAdmissionInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Client.ReadError != "" || run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini semantic failure lost complete response or exact stdout")
	}
	decoder := sse.NewStrictDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(run.Client.Body)), decoder.Finish()...)
	if decoder.Err() != nil || len(frames) < 4 {
		return errors.New("Gemini semantic failure lost SSE framing")
	}
	var responseID, itemID string
	var createdAt int64
	for _, frame := range frames {
		var event struct {
			Type      string              `json:"type"`
			CreatedAt int64               `json:"created_at"`
			Response  struct{ ID string } `json:"response"`
			Item      struct{ ID string } `json:"item"`
		}
		if json.Unmarshal(frame, &event) != nil {
			return errors.New("Gemini semantic failure emitted invalid JSON")
		}
		if event.Type == "response.created" {
			responseID, createdAt = event.Response.ID, event.CreatedAt
		}
		if event.Type == "response.output_item.added" {
			itemID = event.Item.ID
		}
	}
	if createdAt <= 0 || !strings.HasPrefix(responseID, "resp_deepseek_") || !strings.HasPrefix(itemID, "msg_gemini_") || run.Client.Body != geminiStreamToolAdmissionWire04384(scenario, responseID, itemID, createdAt) {
		return errors.New("Gemini semantic failure changed exact rejection or subsequent text")
	}
	run.Client.Body = geminiChatSSEFixture043711(responseID, itemID, createdAt)
	run.Stdout = run.Client.Body + "\n"
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiStreamToolAdmissionNegativeControl04384() bool {
	for _, scenario := range geminiStreamToolAdmissionCases04384() {
		good := geminiStreamToolAdmissionExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.ExitCode = 99 },
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.ReadError = "wrong failure" },
			func(r *geminiCandidateResult) { r.Client.Body = strings.ReplaceAll(r.Client.Body, "ok<", "forged<") },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, "response.failed", "response.completed")
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"sequence_number":0`, `"sequence_number":9`, 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiStreamToolAdmissionExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
