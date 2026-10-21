package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
	"github.com/google/uuid"
)

func TestProdex04384GeminiStreamReplacesRepeatedToolArguments(t *testing.T) {
	for _, tc := range []struct {
		name, first, next string
		position          int
		want              []string
	}{
		{"explicit", `{"id":"call-a","name":"read_file","args":{"path":"partial"}}`, `{"id":"call-a","name":"read_file","args":{"path":"final"}}`, 0, []string{`{"path":"final"}`}},
		{"anonymous", `{"name":"read_file","args":{"path":"partial"}}`, `{"name":"read_file","args":{"path":"final"}}`, 0, []string{`{"path":"final"}`}},
		{"moved-position", `{"name":"read_file","args":{"path":"partial"}}`, `{"name":"read_file","args":null}`, 3, []string{`null`}},
		{"padded-ID", `{"id":" call-a ","name":"read_file","args":1}`, `{"id":" call-a ","name":"read_file"}`, 1, []string{`{}`}},
		{"blank-ID", `{"id":"","name":"read_file","args":1}`, `{"id":"\u2003","name":"read_file","args":2}`, 0, []string{`2`}},
		{"distinct-ID-control", `{"id":"call-a","name":"read_file","args":1}`, `{"id":"call-b","name":"read_file","args":2}`, 0, []string{`1`, `2`}},
		{"explicit-owner-control", `{"id":"call-a","name":"read_file","args":1}`, `{"name":"read_file","args":2}`, 0, []string{`1`, `2`}},
		{"late-explicit-control", `{"name":"read_file","args":1}`, `{"id":"call-b","name":"read_file","args":2}`, 0, []string{`1`, `2`}},
		{"distinct-name-control", `{"name":"read_file","args":1}`, `{"name":"write_file","args":2}`, 0, []string{`1`, `2`}},
		{"canonical-name", `{"name":"RUN-SHELL-COMMAND","args":{"cmd":"p"}}`, `{"name":"exec_command","args":{"cmd":"pwd"}}`, 0, []string{`{"cmd":"pwd"}`}},
		{"signature-retained", `{"id":"call-a","name":"read_file","thoughtSignature":"signed","args":1}`, `{"id":"call-a","name":"read_file","args":2}`, 0, []string{`2`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := geminiStreamReplay04384(t, geminiToolChunk04384(t, 0, tc.first), geminiToolChunk04384(t, tc.position, tc.next))
			var deltas []map[string]any
			var items []any
			completions := 0
			for _, event := range events {
				if event["type"] == "response.function_call_arguments.delta" {
					deltas = append(deltas, event)
				}
				if event["type"] == "response.completed" {
					completions++
					items, _ = event["response"].(map[string]any)["output"].([]any)
				}
			}
			if completions != 1 || len(deltas) != 2 || len(items) != len(tc.want) {
				t.Fatalf("tool updates duplicated/lost output: completions=%d deltas=%#v output=%#v", completions, deltas, items)
			}
			for index, raw := range items {
				item := raw.(map[string]any)
				if item["arguments"] != tc.want[index] {
					t.Errorf("completed args=%#v want=%q", item, tc.want[index])
				}
			}
			if len(tc.want) == 1 && (deltas[0]["call_id"] != deltas[1]["call_id"] || deltas[1]["call_id"] != items[0].(map[string]any)["call_id"]) {
				t.Errorf("one tool changed identity across chunks: %#v output=%#v", deltas, items)
			}
			for _, event := range deltas {
				if id := event["call_id"].(string); strings.HasPrefix(id, "call_gemini_") {
					parsed, err := uuid.Parse(strings.TrimPrefix(id, "call_gemini_"))
					if err != nil || parsed.Version() != 7 {
						t.Errorf("fallback ID must be UUIDv7: %q, %v", id, err)
					}
				}
			}
			if tc.name == "signature-retained" && items[0].(map[string]any)["gemini_thought_signature"] != "signed" {
				t.Errorf("later unsigned snapshot erased continuation signature: %#v", items)
			}
		})
	}
}

func TestProdex04384GeminiStreamToolOutputUsesSlotOrder(t *testing.T) {
	events := geminiStreamReplay04384(t,
		geminiToolChunk04384(t, 3, `{"name":"read_file","id":"call-high","args":1}`),
		geminiToolChunk04384(t, 0, `{"name":"read_file","id":"call-low","args":2}`),
		geminiToolChunk04384(t, 0, `{"name":"read_file","id":"call-third","args":3}`),
		geminiToolChunk04384(t, 8, `{"name":"read_file","id":"call-high","args":4}`))
	last := events[len(events)-1]["response"].(map[string]any)["output"].([]any)
	var ids []any
	for _, raw := range last {
		ids = append(ids, raw.(map[string]any)["call_id"])
	}
	if !reflect.DeepEqual(ids, []any{"call-low", "call-third", "call-high"}) || last[2].(map[string]any)["arguments"] != "4" {
		t.Fatalf("sparse slots or explicit ID binding lost: %#v", last)
	}
}

func TestProdex04384GeminiCompletedStreamIgnoresLaterChunks(t *testing.T) {
	events := geminiStreamReplay04384(t, "[DONE]", geminiToolChunk04384(t, 0, `{"name":"read_file","args":{}}`))
	if len(events) != 1 || events[0]["type"] != "response.completed" {
		t.Fatalf("completed stream emitted a later tool or terminal event: %#v", events)
	}
}

func geminiToolChunk04384(t *testing.T, position int, call string) string {
	t.Helper()
	parts := make([]any, position+1)
	parts[position] = map[string]any{"functionCall": json.RawMessage(call)}
	wire, err := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": parts}}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(wire)
}

func geminiStreamReplay04384(t *testing.T, chunks ...string) []map[string]any {
	t.Helper()
	wire := "data: " + strings.Join(chunks, "\n\ndata: ") + "\n\ndata: [DONE]\n\n"
	response, err := translateResponse(&http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	output, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoder := sse.NewDecoder(1 << 20)
	var events []map[string]any
	for _, data := range append(decoder.Feed(output), decoder.Finish()...) {
		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}
