package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestProdex04384GeminiNativeToolItemLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, call string
		want       []string
	}{
		{"ordinary", `{"name":"read_file","id":"call-one","args":{"path":"final"}}`, []string{"response.output_item.added", "response.function_call_arguments.delta", "response.function_call_arguments.delta", "response.output_item.done", "response.completed"}},
		{"search", `{"name":"tool_search","id":"call-one","args":null}`, []string{"response.output_item.done", "response.completed"}},
		{"patch", `{"name":"apply_patch","id":"call-one","args":{"input":"final"}}`, []string{"response.output_item.done", "response.completed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := geminiToolChunk04384(t, 0, tc.call)
			events := geminiStreamReplay04384(t, first, first)
			var kinds []string
			var done map[string]any
			for index, event := range events {
				kinds = append(kinds, event["type"].(string))
				if event["sequence_number"] != float64(index) {
					t.Errorf("tool lifecycle sequence skipped: %#v", events)
				}
				if event["type"] == "response.output_item.added" {
					item := event["item"].(map[string]any)
					if _, present := item["arguments"]; present || item["call_id"] != "call-one" {
						t.Errorf("added item exposed final args or wrong ID: %#v", event)
					}
				}
				if event["type"] == "response.output_item.done" {
					done = event["item"].(map[string]any)
					if _, present := event["response_id"]; present {
						t.Errorf("native tool done acquired a foreign response_id: %#v", event)
					}
				}
			}
			if !reflect.DeepEqual(kinds, tc.want) {
				t.Fatalf("tool lifecycle=%v want=%v", kinds, tc.want)
			}
			output := events[len(events)-1]["response"].(map[string]any)["output"].([]any)
			if len(output) != 1 || !reflect.DeepEqual(output[0], done) {
				t.Errorf("completed output disagrees with the tool done event: %#v", output)
			}
		})
	}
}

func TestProdex04384GeminiNativeEventUsesCRLFFraming(t *testing.T) {
	var wire bytes.Buffer
	if err := writeGeminiSSEEvent(&wire, "response.output_item.done", map[string]any{
		"type": "response.output_item.done", "sequence_number": 2,
		"item": map[string]any{"type": "function_call", "call_id": "call-one", "arguments": "<>&"},
	}); err != nil {
		t.Fatal(err)
	}
	want := "event: response.output_item.done\r\ndata: " + `{"item":{"arguments":"<>&","call_id":"call-one","type":"function_call"},"sequence_number":2,"type":"response.output_item.done"}` + "\r\n\r\n"
	if wire.String() != want {
		t.Fatalf("native SSE event bytes=%q want=%q", wire.String(), want)
	}
}

func TestProdex04384GeminiFinishWaitsForDoneAndKeepsTrailingUsage(t *testing.T) {
	chunk := `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call-one","name":"read_file","args":{}}}]},"finishReason":"STOP"}]}`
	state := &geminiStreamState{responseID: "resp-finish"}
	var wire bytes.Buffer
	if err := state.consume(&wire, []byte(chunk), nil); err != nil {
		t.Fatal(err)
	}
	if state.completed || strings.Contains(wire.String(), "event: response.completed") || !strings.Contains(wire.String(), "event: response.output_item.done") {
		t.Errorf("finish reason did not complete only the pending tool: %s", wire.String())
	}
	events := geminiStreamReplay04384(t, chunk, `{"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`)
	last := events[len(events)-1]["response"].(map[string]any)
	usage, _ := last["usage"].(map[string]any)
	if usage["total_tokens"] != float64(5) {
		t.Errorf("STOP discarded trailing usage: %#v", last)
	}
	response, err := translateResponse(&http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\n"))}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !bytes.Contains(body, []byte("event: response.failed")) || bytes.Contains(body, []byte("event: response.completed")) {
		t.Errorf("premature EOF after STOP became success: %s, %v", body, err)
	}
}

func TestProdex04384GeminiTerminalFinishKinds(t *testing.T) {
	for _, tc := range []struct{ reason, terminal, code string }{
		{"MAX_TOKENS", "response.incomplete", "max_output_tokens"},
		{"MALFORMED_FUNCTION_CALL", "response.failed", "gemini_malformed_function_call"},
		{"SAFETY", "response.failed", "invalid_prompt"},
		{"OTHER", "response.failed", "gemini_finish_other"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			reason, _ := json.Marshal(tc.reason)
			events := geminiStreamReplay04384(t, `{"candidates":[{"content":{"parts":[{"text":"partial"}]},"finishReason":`+string(reason)+`}]}`)
			last := events[len(events)-1]
			if last["type"] != tc.terminal {
				t.Fatalf("finish %s terminal=%#v", tc.reason, last)
			}
			body, _ := json.Marshal(last)
			if !bytes.Contains(body, []byte(tc.code)) {
				t.Errorf("finish %s lost error/incomplete reason: %s", tc.reason, body)
			}
		})
	}
}

func TestProdex04384GeminiFinishRequiresCandidatePartsArray(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		terminal      bool
	}{
		{"missing", "", false},
		{"null-content", `,"content":null`, false},
		{"missing-parts", `,"content":{}`, false},
		{"null-parts", `,"content":{"parts":null}`, false},
		{"object-parts", `,"content":{"parts":{}}`, false},
		{"empty-array", `,"content":{"parts":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &geminiStreamState{responseID: "resp-parts"}
			var output bytes.Buffer
			chunk := `{"candidates":[{"finishReason":"MALFORMED_FUNCTION_CALL"` + tc.content + `}]}`
			if err := state.consume(&output, []byte(chunk), nil); err != nil {
				t.Fatal(err)
			}
			if state.completed != tc.terminal || state.finish != "MALFORMED_FUNCTION_CALL" {
				t.Errorf("finish without parts array: completed=%v finish=%q output=%s", state.completed, state.finish, output.String())
			}
		})
	}
}

func TestProdex04384GeminiFinishedToolSlotsAreNotCompletedTwice(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		member := ""
		if explicit {
			member = `"id":"call-bound",`
		}
		first := `{"candidates":[{"content":{"parts":[{"functionCall":{` + member + `"name":"read_file","args":1}}]},"finishReason":"STOP"}]}`
		next := geminiToolChunk04384(t, 0, `{`+member+`"name":"read_file","args":2}`)
		events := geminiStreamReplay04384(t, first, next)
		count := 0
		for _, event := range events {
			if event["type"] == "response.output_item.done" {
				count++
			}
		}
		want := 2
		if explicit {
			want = 1
		}
		output := events[len(events)-1]["response"].(map[string]any)["output"].([]any)
		if count != want || len(output) != want || output[len(output)-1].(map[string]any)["arguments"] != "2" {
			t.Errorf("done-slot ownership explicit=%v count=%d output=%#v", explicit, count, output)
		}
	}
}
