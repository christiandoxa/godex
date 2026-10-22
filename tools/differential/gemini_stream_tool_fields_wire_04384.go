package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
	"github.com/google/uuid"
)

func geminiToolFieldsWire04384(scenario, responseID, fallback string) string {
	f := geminiToolFieldsFixtureFor04384(scenario)
	if f.fallback && fallback != "" {
		f.initialID, f.finalID = fallback, fallback
	}
	added := map[string]any{"type": "function_call", "call_id": f.initialID, "name": f.initialName}
	item := map[string]any{"type": "function_call", "call_id": f.finalID, "name": f.finalName, "arguments": "{}"}
	if f.namespace != "" {
		added["namespace"], item["namespace"] = f.namespace, f.namespace
	}
	if f.signature != "" {
		item["gemini_thought_signature"] = f.signature
	}
	events := []map[string]any{
		{"type": "response.created", "created_at": 1, "response": map[string]any{"id": responseID}},
		{"type": "response.output_item.added", "output_index": f.index, "item": added},
		{"type": "response.function_call_arguments.delta", "output_index": f.index, "call_id": f.initialID, "delta": "{}"},
		{"type": "response.output_item.done", "output_index": f.index, "item": item},
		{"type": "response.completed", "created_at": 1, "response": map[string]any{"id": responseID, "model": "gemini-3.1-pro-preview", "output": []any{item}, "metadata": map[string]any{"gemini": map[string]any{"finish_reason": "tool_calls"}}, "usage": map[string]any{"input_tokens": uint64(9007199254740993), "output_tokens": 1, "total_tokens": uint64(9007199254740994)}}},
	}
	var wire strings.Builder
	for sequence, event := range events {
		event["sequence_number"] = sequence
		encoded, _ := json.Marshal(event)
		canonical := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&", `\u2028`, "\u2028", `\u2029`, "\u2029").Replace(string(encoded))
		_, _ = fmt.Fprintf(&wire, "event: %s\r\ndata: %s\r\n\r\n", event["type"], canonical)
	}
	return wire.String()
}

func geminiToolFieldsExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	run.Client.Body = geminiToolFieldsWire04384(scenario, "resp_deepseek_fixture", "")
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiToolFieldsInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Client.ReadError != "" || run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini tool fields lost full read or exact stdout")
	}
	decoder := sse.NewStrictDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(run.Client.Body)), decoder.Finish()...)
	if decoder.Err() != nil || len(frames) != 5 {
		return errors.New("Gemini tool fields lost exact SSE lifecycle")
	}
	var created struct {
		Response struct{ ID string } `json:"response"`
	}
	var added struct {
		Item struct {
			CallID string `json:"call_id"`
		}
	}
	if json.Unmarshal(frames[0], &created) != nil || json.Unmarshal(frames[1], &added) != nil || !strings.HasPrefix(created.Response.ID, "resp_deepseek_") {
		return errors.New("Gemini tool fields lost response identity")
	}
	fallback := ""
	if geminiToolFieldsFixtureFor04384(scenario).fallback {
		fallback = added.Item.CallID
		parsed, err := uuid.Parse(strings.TrimPrefix(fallback, "call_deepseek_"))
		if err != nil || !strings.HasPrefix(fallback, "call_deepseek_") || parsed.Version() != 7 || parsed.Variant() != uuid.RFC4122 || fallback != "call_deepseek_"+parsed.String() {
			return errors.New("Gemini tool fallback lost exact prefix or canonical UUIDv7")
		}
	}
	if run.Client.Body != geminiToolFieldsWire04384(scenario, created.Response.ID, fallback) {
		return errors.New("Gemini tool fields changed exact IDs, names, signatures or indices")
	}
	run.Client.Body = geminiChatSSEFixture043711(created.Response.ID, "msg_gemini_fixture", 1)
	run.Stdout = run.Client.Body + "\n"
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiToolFieldsNegativeControl04384() bool {
	for _, scenario := range geminiToolFieldsCases04384() {
		good := geminiToolFieldsExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.ReadError = "wrong failure" },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"sequence_number":0`, `"sequence_number":9`, 1)
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, "function_call", "wrong_type")
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, `"call_id":`, `"wrong_call_id":`)
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, `"name":`, `"wrong_name":`)
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, `"output_index":`, `"wrong_index":`)
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"arguments":"{}"`, `"gemini_thought_signature":"forged","arguments":"{}"`, 1)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiToolFieldsExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
