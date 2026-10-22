package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

const geminiStreamReasoning04384 = "synthetic reasoning <>&"

func geminiReasoningStreamCases04384() []string {
	return []string{"stream-chat-reasoning", "stream-chat-reasoning-mixed", "stream-chat-reasoning-empty", "stream-chat-reasoning-only"}
}

func geminiReasoningStreamCase04384(scenario string) bool {
	return slices.Contains(geminiReasoningStreamCases04384(), scenario)
}

func geminiReasoningStreamFixture04384(scenario, responseID, itemID string, epoch int64) string {
	base := geminiChatSSEFixture043711(responseID, itemID, epoch)
	if scenario == "stream-chat-reasoning-empty" {
		return base
	}
	decoder := sse.NewDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(base)), decoder.Finish()...)
	var events []map[string]any
	for i, frame := range frames {
		var event map[string]any
		parser := json.NewDecoder(strings.NewReader(string(frame)))
		parser.UseNumber()
		_ = parser.Decode(&event)
		if i == 1 {
			events = append(events,
				map[string]any{"type": "response.reasoning_summary_part.added", "response_id": responseID, "output_index": 0, "summary_index": 0, "part": map[string]any{"type": "summary_text", "text": ""}},
				map[string]any{"type": "response.reasoning_summary_text.delta", "response_id": responseID, "output_index": 0, "summary_index": 0, "delta": geminiStreamReasoning04384},
			)
		}
		if i == 4 {
			events = append(events, map[string]any{"type": "response.reasoning_summary_text.done", "response_id": responseID, "output_index": 0, "summary_index": 0, "text": geminiStreamReasoning04384})
		}
		if i == 5 {
			event["response"].(map[string]any)["metadata"].(map[string]any)["gemini"].(map[string]any)["reasoning_content"] = geminiStreamReasoning04384
			if scenario == "stream-chat-reasoning-only" {
				event["response"].(map[string]any)["output"] = []any{}
			}
		}
		if scenario == "stream-chat-reasoning-only" && i > 0 && i < 5 {
			continue
		}
		events = append(events, event)
	}
	var wire strings.Builder
	for i, event := range events {
		event["sequence_number"] = i
		body, _ := json.Marshal(event)
		canonical := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&", `\u2028`, "\u2028", `\u2029`, "\u2029").Replace(string(body))
		_, _ = fmt.Fprintf(&wire, "event: %s\r\ndata: %s\r\n\r\n", event["type"], canonical)
	}
	return wire.String()
}

func geminiReasoningStreamWireValid04384(scenario, wire string) bool {
	decoder := sse.NewStrictDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(wire)), decoder.Finish()...)
	if decoder.Err() != nil || len(frames) < 2 {
		return false
	}
	var created struct {
		CreatedAt int64               `json:"created_at"`
		Response  struct{ ID string } `json:"response"`
	}
	if json.Unmarshal(frames[0], &created) != nil || created.CreatedAt <= 0 || !strings.HasPrefix(created.Response.ID, "resp_deepseek_") {
		return false
	}
	var itemID string
	for _, frame := range frames {
		var event struct {
			Type string              `json:"type"`
			Item struct{ ID string } `json:"item"`
		}
		if json.Unmarshal(frame, &event) == nil && event.Type == "response.output_item.added" {
			itemID = event.Item.ID
			break
		}
	}
	return (scenario == "stream-chat-reasoning-only" || strings.HasPrefix(itemID, "msg_gemini_")) && wire == geminiReasoningStreamFixture04384(scenario, created.Response.ID, itemID, created.CreatedAt)
}

func geminiReasoningStreamExpected04384(scenario string) geminiCandidateResult {
	run := geminiUserIDExpected04384("request-user-id-absent")
	run.Client.Body = geminiReasoningStreamFixture04384(scenario, "resp_deepseek_fixture", "msg_gemini_fixture", 111)
	run.Client.Headers = http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}}
	run.Stdout = run.Client.Body + "\n"
	run.Upstream[0].RawBody = `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":true,"stream_options":{"include_usage":true}}`
	_ = json.Unmarshal([]byte(run.Upstream[0].RawBody), &run.Upstream[0].Body)
	return run
}

func geminiReasoningStreamInvariant04384(scenario string, run geminiCandidateResult) error {
	want := geminiReasoningStreamExpected04384(scenario)
	if run.ClientError != "" || len(run.StateIntegrity) != 0 || run.ExitCode != 0 || run.Client.Status != 200 || run.Client.Headers.Get("Content-Type") != want.Client.Headers.Get("Content-Type") || run.Stdout != run.Client.Body+"\n" || len(run.Upstream) != 1 {
		return errors.New("Gemini reasoning stream lost response, attempts, stdout or state")
	}
	request := run.Upstream[0]
	if request.RawBody != want.Upstream[0].RawBody || request.Path != "/v1beta/openai/chat/completions" || request.Method != "POST" || request.Slot != "single" || !request.Auth {
		return errors.New("Gemini reasoning stream changed original request or credential")
	}
	if !geminiReasoningStreamWireValid04384(scenario, run.Client.Body) {
		return errors.New("Gemini reasoning stream changed exact lifecycle or completed metadata")
	}
	return nil
}

func geminiReasoningStreamNegativeControl04384() bool {
	for _, scenario := range geminiReasoningStreamCases04384() {
		good := geminiReasoningStreamExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for index, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, "synthetic", "wrong")
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, "sequence_number", "wrong")
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, "reasoning_content", "wrong")
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			if scenario == "stream-chat-reasoning-empty" && index == 3 {
				continue
			}
			run := geminiReasoningStreamExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
