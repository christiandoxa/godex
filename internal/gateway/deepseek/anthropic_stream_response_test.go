package deepseek

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestAnthropicStreamReasoningEventsUseResponsesIndexZero(t *testing.T) {
	now := time.Unix(1, 2)
	state := anthropicStreamState{}
	created, supported, err := state.translate([]byte(`{"type":"message_start","message":{"model":"deepseek-v4-pro"}}`), now)
	if err != nil || !supported {
		t.Fatalf("message_start = %q / %v / %v", created, supported, err)
	}
	if !strings.HasPrefix(state.id, "resp_deepseek_") {
		t.Fatalf("fallback response ID = %q", state.id)
	}
	other := anthropicStreamState{}
	if _, _, err := other.translate([]byte(`{"type":"message_start","message":{}}`), now); err != nil {
		t.Fatal(err)
	}
	if other.id == state.id {
		t.Fatalf("fallback response IDs are not unique: %q", state.id)
	}
	started, supported, err := state.translate([]byte(`{"type":"content_block_start","index":5,"content_block":{"type":"thinking","thinking":""}}`), now)
	if err != nil || supported || len(started) != 0 {
		t.Fatalf("thinking block start = %q / %v / %v", started, supported, err)
	}
	delta, supported, err := state.translate([]byte(`{"type":"content_block_delta","index":5,"delta":{"type":"thinking_delta","thinking":"considering"}}`), now)
	if err != nil || !supported {
		t.Fatalf("thinking delta = %q / %v / %v", delta, supported, err)
	}
	completed, supported, err := state.translate([]byte(`{"type":"message_stop"}`), now)
	if err != nil || !supported {
		t.Fatalf("message_stop = %q / %v / %v", completed, supported, err)
	}

	var stream strings.Builder
	stream.Write(created)
	stream.Write(delta)
	stream.Write(completed)
	decoder := sse.NewDecoder(streamEventMaxBytes)
	var events []map[string]any
	for _, data := range decoder.Feed([]byte(stream.String())) {
		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	want := []string{
		"response.created", "response.reasoning_summary_part.added",
		"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.completed",
	}
	if len(events) != len(want) {
		t.Fatalf("reasoning events = %#v", events)
	}
	for index, kind := range want {
		if events[index]["type"] != kind {
			t.Fatalf("event %d type = %#v, want %q", index, events[index]["type"], kind)
		}
	}
	for _, event := range events[1:4] {
		if event["output_index"] != float64(0) {
			t.Errorf("reasoning output_index = %#v", event["output_index"])
		}
	}
	if events[2]["summary_index"] != float64(0) || events[2]["delta"] != "considering" || events[3]["text"] != "considering" {
		t.Fatalf("reasoning lifecycle = %#v", events[1:4])
	}
}

func TestAnthropicStreamOmitsEmptyTextAndPreservesWhitespace(t *testing.T) {
	for _, test := range []struct {
		name       string
		delta      string
		wantDelta  bool
		wantOutput int
		wantText   string
	}{
		{name: "empty", delta: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":""}}`},
		{name: "whitespace", delta: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" "}}`, wantDelta: true, wantOutput: 1, wantText: " "},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Unix(123, 0)
			state := anthropicStreamState{}
			if _, _, err := state.translate([]byte(`{"type":"message_start","message":{"id":"msg_text"}}`), now); err != nil {
				t.Fatal(err)
			}
			if _, _, err := state.translate([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`), now); err != nil {
				t.Fatal(err)
			}
			delta, emitted, err := state.translate([]byte(test.delta), now)
			if err != nil || emitted != test.wantDelta || (len(delta) > 0) != test.wantDelta {
				t.Fatalf("text delta = %q / emitted:%t / err:%v", delta, emitted, err)
			}
			if test.wantText != "" && !strings.Contains(string(delta), `"delta":" "`) {
				t.Fatalf("whitespace delta was lost: %s", delta)
			}

			completed, emitted, err := state.translate([]byte(`{"type":"message_stop"}`), now)
			if err != nil || !emitted {
				t.Fatalf("message_stop = %q / emitted:%t / err:%v", completed, emitted, err)
			}
			var response map[string]any
			for _, data := range sse.NewDecoder(streamEventMaxBytes).Feed(completed) {
				var event map[string]any
				if err := json.Unmarshal(data, &event); err != nil {
					t.Fatal(err)
				}
				if event["type"] == "response.completed" {
					response = event["response"].(map[string]any)
				}
			}
			output := response["output"].([]any)
			if len(output) != test.wantOutput {
				t.Fatalf("completed output = %#v", output)
			}
			if test.wantText != "" {
				content := output[0].(map[string]any)["content"].([]any)
				if got := content[0].(map[string]any)["text"]; got != test.wantText {
					t.Fatalf("completed text = %#v", got)
				}
			}
		})
	}
}

func TestAnthropicResponseToolNameRestoresNamespace(t *testing.T) {
	response, err := deepSeekAnthropicResponse([]byte(`{
		"id":"msg_tool","content":[{"type":"tool_use","id":"call_tool","name":"files--read_file","input":{}}]
	}`), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(response, &value); err != nil {
		t.Fatal(err)
	}
	tool := value["output"].([]any)[0].(map[string]any)
	if tool["namespace"] != "files" || tool["name"] != "read_file" {
		t.Fatalf("Anthropic namespaced tool = %#v", tool)
	}
}
