package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func geminiLifecycleWire04384(scenario, responseID, itemID string) string {
	decoder := sse.NewDecoder(1 << 20)
	baseWire := geminiChatSSEFixture043711(responseID, itemID, 1)
	frames := append(decoder.Feed([]byte(baseWire)), decoder.Finish()...)
	var base []map[string]any
	for _, frame := range frames {
		var event map[string]any
		parser := json.NewDecoder(bytes.NewReader(frame))
		parser.UseNumber()
		_ = parser.Decode(&event)
		base = append(base, event)
	}
	events := []map[string]any{base[0]}
	mixed := scenario == "stream-chat-tool-mixed-text"
	if mixed {
		events = append(events, base[1], base[2])
	}
	tools := geminiLifecycleTools04384(scenario)
	for _, tool := range tools {
		name := tool.name
		if name == "" {
			name = "tool_call"
		}
		item := map[string]any{"type": "function_call", "call_id": tool.id, "name": name}
		if name == "apply_patch" {
			item = map[string]any{"type": "custom_tool_call", "call_id": tool.id, "name": name, "input": ""}
		}
		if name != "tool_search" {
			events = append(events, map[string]any{"type": "response.output_item.added", "output_index": tool.index, "item": item})
		}
		fragments := tool.fragments
		if len(fragments) == 0 {
			fragments = []string{tool.arguments}
		}
		for _, fragment := range fragments {
			if fragment != "" && name != "tool_search" && name != "apply_patch" {
				events = append(events, map[string]any{"type": "response.function_call_arguments.delta", "output_index": tool.index, "call_id": tool.id, "delta": fragment})
			}
		}
	}
	if mixed {
		events = append(events, base[3])
	}
	invalid := scenario == "stream-chat-tool-invalid-arguments" || scenario == "stream-chat-tool-missing-name"
	if invalid {
		message := "Gemini OpenAI-compatible streamed malformed JSON arguments for tool call `lookup` at index 2: invalid JSON"
		if scenario == "stream-chat-tool-missing-name" {
			message = "Gemini OpenAI-compatible streamed a tool call without a function name at index 2"
		}
		events = append(events, map[string]any{"type": "response.failed", "created_at": 1, "response": map[string]any{"id": responseID, "error": map[string]any{"code": "invalid_tool_call_arguments", "message": message}}})
	} else {
		sort.Slice(tools, func(i, j int) bool { return tools[i].index < tools[j].index })
		output := []any{}
		completed := base[5]["response"].(map[string]any)
		if mixed {
			output = append(output, completed["output"].([]any)[0])
		}
		for _, tool := range tools {
			arguments := tool.arguments
			if tool.name == "shell" {
				arguments = `{"cmd":"rtk git status"}`
			}
			item := map[string]any{"type": "function_call", "call_id": tool.id, "name": tool.name, "arguments": arguments}
			switch tool.name {
			case "tool_search":
				item = map[string]any{"type": "tool_search_call", "call_id": tool.id, "execution": "client", "arguments": map[string]any{"q": "synthetic"}}
			case "apply_patch":
				item = map[string]any{"type": "custom_tool_call", "call_id": tool.id, "name": tool.name, "input": "synthetic patch"}
			}
			events = append(events, map[string]any{"type": "response.output_item.done", "output_index": tool.index, "item": item})
			output = append(output, item)
		}
		if mixed {
			events = append(events, base[4])
		} else {
			completed["metadata"].(map[string]any)["gemini"].(map[string]any)["finish_reason"] = "tool_calls"
		}
		completed["output"] = output
		events = append(events, base[5])
	}
	var wire strings.Builder
	for index, event := range events {
		event["sequence_number"] = index
		encoded, _ := json.Marshal(event)
		canonical := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&", `\u2028`, "\u2028", `\u2029`, "\u2029").Replace(string(encoded))
		_, _ = fmt.Fprintf(&wire, "event: %s\r\ndata: %s\r\n\r\n", event["type"], canonical)
	}
	return wire.String()
}

func geminiLifecycleExpected04384(scenario string) geminiCandidateResult {
	run := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	run.Client.Body = geminiLifecycleWire04384(scenario, "resp_deepseek_fixture", "msg_gemini_fixture")
	run.Stdout = run.Client.Body + "\n"
	return run
}

func geminiLifecycleInvariant04384(scenario string, run geminiCandidateResult) error {
	if run.Client.ReadError != "" || run.Stdout != run.Client.Body+"\n" {
		return errors.New("Gemini tool lifecycle lost completed read or exact stdout")
	}
	decoder := sse.NewStrictDecoder(1 << 20)
	frames := append(decoder.Feed([]byte(run.Client.Body)), decoder.Finish()...)
	var responseID, itemID string
	for _, frame := range frames {
		var event struct {
			Type     string              `json:"type"`
			Response struct{ ID string } `json:"response"`
			Item     struct{ ID string } `json:"item"`
		}
		if json.Unmarshal(frame, &event) != nil {
			return errors.New("Gemini tool lifecycle lost valid JSON")
		}
		if event.Type == "response.created" {
			responseID = event.Response.ID
		}
		if event.Type == "response.output_item.added" && event.Item.ID != "" {
			itemID = event.Item.ID
		}
	}
	if decoder.Err() != nil || !strings.HasPrefix(responseID, "resp_deepseek_") || (scenario == "stream-chat-tool-mixed-text" && !strings.HasPrefix(itemID, "msg_gemini_")) || run.Client.Body != geminiLifecycleWire04384(scenario, responseID, itemID) {
		return errors.New("Gemini tool lifecycle changed exact item, index, arguments or terminal wire")
	}
	run.Client.Body = geminiChatSSEFixture043711(responseID, "msg_gemini_fixture", 1)
	run.Stdout = run.Client.Body + "\n"
	return geminiReasoningStreamInvariant04384("stream-chat-reasoning-empty", run)
}

func geminiLifecycleNegativeControl04384() bool {
	for _, scenario := range geminiStreamToolLifecycleCases04384() {
		good := geminiLifecycleExpected04384(scenario)
		if len(geminiCompare(scenario, good, good)) != 0 {
			return false
		}
		for _, mutate := range []func(*geminiCandidateResult){
			func(r *geminiCandidateResult) { r.ExitCode++ },
			func(r *geminiCandidateResult) { r.Client.Status++ },
			func(r *geminiCandidateResult) { r.Client.ReadError = "wrong failure" },
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, "call_synthetic", "forged_call")
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.Replace(r.Client.Body, `"sequence_number":0`, `"sequence_number":9`, 1)
			},
			func(r *geminiCandidateResult) {
				r.Client.Body = strings.ReplaceAll(r.Client.Body, `"output_index":2`, `"output_index":99`)
			},
			func(r *geminiCandidateResult) { r.Upstream[0].RawBody = `{}` },
			func(r *geminiCandidateResult) { r.Upstream[0].Auth = false },
			func(r *geminiCandidateResult) { r.Upstream = append(r.Upstream, r.Upstream[0]) },
			func(r *geminiCandidateResult) { r.StateIntegrity = []string{"wrong"} },
		} {
			run := geminiLifecycleExpected04384(scenario)
			mutate(&run)
			run.Stdout = run.Client.Body + "\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
