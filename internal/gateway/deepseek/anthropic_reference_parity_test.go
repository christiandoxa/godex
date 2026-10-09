package deepseek

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestAnthropicReferenceResponseEnvelopeFixtures(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		check func(*testing.T, map[string]any)
	}{
		{
			name: "defaults",
			body: `{"content":[]}`,
			check: func(t *testing.T, got map[string]any) {
				if got["id"] != "resp_anthropic" || got["model"] != "unknown" || got["created_at"] != float64(123) {
					t.Fatalf("default envelope = %#v", got)
				}
			},
		},
		{
			name: "usage search and null stop reason",
			body: `{"id":"msg_test","model":"claude","content":[],"usage":{"input_tokens":9,"output_tokens":4,"server_tool_use":{"web_search_requests":2}},"stop_reason":null}`,
			check: func(t *testing.T, got map[string]any) {
				usage := got["usage"].(map[string]any)
				if usage["input_tokens"] != float64(9) || usage["output_tokens"] != float64(4) || usage["total_tokens"] != float64(13) {
					t.Fatalf("usage = %#v", usage)
				}
				if got["tool_usage"].(map[string]any)["web_search"].(map[string]any)["num_requests"] != float64(2) {
					t.Fatalf("tool usage = %#v", got["tool_usage"])
				}
				if value, ok := got["metadata"].(map[string]any)["anthropic"].(map[string]any)["stop_reason"]; !ok || value != nil {
					t.Fatalf("stop reason = %#v", got["metadata"])
				}
			},
		},
		{
			name: "saturating usage",
			body: `{"id":null,"model":1,"content":[],"usage":{"input_tokens":18446744073709551615,"output_tokens":1},"stop_reason":"end_turn"}`,
			check: func(t *testing.T, got map[string]any) {
				if got["id"] != "resp_anthropic" || got["model"] != "unknown" {
					t.Fatalf("fallback identity = %#v", got)
				}
				usage := got["usage"].(map[string]any)
				if usage["input_tokens"] != float64(math.MaxUint64) || usage["output_tokens"] != float64(1) || usage["total_tokens"] != float64(math.MaxUint64) {
					t.Fatalf("saturated usage = %#v", usage)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := deepSeekAnthropicResponse([]byte(test.body), time.Unix(123, 0))
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			test.check(t, got)
		})
	}
}

func TestAnthropicReferenceResponseOrderingAndSourceFiltering(t *testing.T) {
	body, err := deepSeekAnthropicResponse([]byte(`{
		"content":[
			{"type":"server_tool_use","id":"srv_duplicate","name":"web_search","input":{"query":"first"}},
			{"type":"server_tool_use","id":"srv_duplicate","name":"web_search","input":{"query":"last"}},
			{"type":"web_search_tool_result","tool_use_id":"srv_duplicate","content":[
				{"url":"https://example.com/one","title":"🦀"},
				{"url":false,"title":"ignored"},
				{"url":"https://example.com/two","title":7},
				"ignored"
			]}
		]
	}`), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	output := got["output"].([]any)
	first := output[0].(map[string]any)["action"].(map[string]any)["sources"].([]any)
	second := output[1].(map[string]any)["action"].(map[string]any)["sources"].([]any)
	if len(first) != 0 || len(second) != 2 {
		t.Fatalf("duplicate source attachment = %#v", output)
	}
	one := second[0].(map[string]any)
	two := second[1].(map[string]any)
	if one["url"] != "https://example.com/one" || one["title"] != "🦀" || two["url"] != "https://example.com/two" {
		t.Fatalf("filtered sources = %#v", second)
	}
	if _, found := two["title"]; found {
		t.Fatalf("non-string title leaked = %#v", two)
	}
}

func TestAnthropicReferenceResponsePreservesEscapedTextAndReasoning(t *testing.T) {
	body, err := deepSeekAnthropicResponse([]byte(`{"content":[
		{"type":"text","text":"line \"one\"\nline two"},
		{"type":"thinking","thinking":"hidden \"step\""},
		{"type":"text","text":"done"}
	]}`), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	output := got["output"].([]any)
	if len(output) != 3 || output[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"] != "line \"one\"\nline two" || output[1].(map[string]any)["summary"].([]any)[0].(map[string]any)["text"] != "hidden \"step\"" || output[2].(map[string]any)["content"].([]any)[0].(map[string]any)["text"] != "done" {
		t.Fatalf("escaped response output = %#v", output)
	}
}

func TestAnthropicReferenceRequestRejectsTrailingJSON(t *testing.T) {
	if _, err := deepSeekAnthropicRequest([]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]} {}")); err == nil {
		t.Fatal("native Messages request accepted trailing JSON")
	}
}

func TestAnthropicReferenceChatRequestFieldPolicy(t *testing.T) {
	for _, field := range []string{"response_format", "user_id", "top_logprobs", "logprobs", "presence_penalty", "frequency_penalty", "seed", "unknown_field"} {
		request := map[string]any{"model": "deepseek-chat", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "stream": false}
		request[field] = true
		body, _ := json.Marshal(request)
		if _, err := deepSeekAnthropicRequest(body); err == nil {
			t.Fatalf("unmapped field %q unexpectedly accepted", field)
		}
	}
	body, err := deepSeekAnthropicRequest([]byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hello"}],"stream":true,"parallel_tool_calls":true,"stream_options":{"include_usage":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "deepseek-chat" || got["stream"] != true {
		t.Fatalf("benign transport request = %#v", got)
	}
}

func TestAnthropicReferenceStreamTranslatesTextWithoutBlockStart(t *testing.T) {
	state := anthropicStreamState{}
	if _, supported, err := state.translate([]byte(`{"type":"message_start","message":{"id":"msg_test","model":"deepseek-chat","usage":{"input_tokens":2,"output_tokens":0}}}`), time.Unix(123, 0)); err != nil || !supported {
		t.Fatalf("message start = supported:%t err:%v", supported, err)
	}
	translated, supported, err := state.translate([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("text delta without start = %q supported:%t err:%v", translated, supported, err)
	}
	if !strings.Contains(string(translated), `"type":"response.output_text.delta"`) || !strings.Contains(string(translated), `"delta":"hello"`) {
		t.Fatalf("translated text delta = %s", translated)
	}
}

func TestAnthropicReferenceStreamPreservesProviderError(t *testing.T) {
	state := anthropicStreamState{}
	translated, supported, err := state.translate([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"try later"}}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("provider error = %q supported:%t err:%v", translated, supported, err)
	}
	if !strings.Contains(string(translated), `"code":"overloaded_error"`) || !strings.Contains(string(translated), `"message":"try later"`) {
		t.Fatalf("provider error translation = %s", translated)
	}
}

func TestAnthropicReferenceStreamFiltersSearchQueriesAndDefaultsID(t *testing.T) {
	state := anthropicStreamState{}
	translated, supported, err := state.translate([]byte(`{"type":"content_block_start","index":5,"content_block":{"type":"server_tool_use","name":"web_search","input":{"queries":["one",null,3,"🦀"]}}}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("search start = %q supported:%t err:%v", translated, supported, err)
	}
	var payload map[string]any
	data := strings.SplitN(string(translated), "data: ", 2)
	if len(data) != 2 || json.Unmarshal([]byte(strings.TrimSpace(data[1])), &payload) != nil {
		t.Fatalf("search event = %s", translated)
	}
	item := payload["item"].(map[string]any)
	if item["id"] != "web_search_5" {
		t.Fatalf("search fallback id = %#v", item)
	}
	queries := item["action"].(map[string]any)["queries"].([]any)
	if len(queries) != 2 || queries[0] != "one" || queries[1] != "🦀" {
		t.Fatalf("filtered search queries = %#v", queries)
	}
}

func TestAnthropicReferenceStreamInvalidSearchJSONCompletesWithEmptyQueries(t *testing.T) {
	state := anthropicStreamState{}
	if _, _, err := state.translate([]byte(`{"type":"message_start","message":{"id":"msg_search_invalid"}}`), time.Unix(123, 0)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.translate([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"srv_invalid","name":"web_search","input":{}}}`), time.Unix(123, 0)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.translate([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"unfinished"}}`), time.Unix(123, 0)); err != nil {
		t.Fatal(err)
	}
	translated, supported, err := state.translate([]byte(`{"type":"message_stop"}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("invalid search completion = %q supported:%t err:%v", translated, supported, err)
	}
	if !strings.Contains(string(translated), `"queries":[]`) || !strings.Contains(string(translated), `"id":"srv_invalid"`) {
		t.Fatalf("invalid search completion = %s", translated)
	}
}

func TestAnthropicReferenceStreamReasoningDeltaWithoutBlockStart(t *testing.T) {
	state := anthropicStreamState{}
	translated, supported, err := state.translate([]byte(`{"type":"content_block_delta","index":4,"delta":{"type":"thinking_delta","thinking":"reasoning"}}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("reasoning delta without start = %q supported:%t err:%v", translated, supported, err)
	}
	if !strings.Contains(string(translated), `response.reasoning_summary_text.delta`) || !strings.Contains(string(translated), `"delta":"reasoning"`) {
		t.Fatalf("reasoning delta = %s", translated)
	}
}

func TestAnthropicReferenceStreamEventUsesRuntimeCRLFFraming(t *testing.T) {
	got := string(anthropicStreamEvent("response.output_text.delta", map[string]any{
		"type": "response.output_text.delta", "delta": "hello",
	}))
	if !strings.HasPrefix(got, "event: response.output_text.delta\r\ndata: ") || !strings.HasSuffix(got, "\r\n\r\n") {
		t.Fatalf("Anthropic runtime SSE framing = %q", got)
	}
}

func TestAnthropicReferenceBufferedRejectsTrailingJSON(t *testing.T) {
	if _, err := deepSeekAnthropicResponse([]byte("{\"content\":[]}{\"extra\":1}"), time.Unix(1, 0)); err == nil {
		t.Fatal("native Messages response accepted trailing JSON")
	}
}

func TestAnthropicReferenceBufferedUsesFourMiBBound(t *testing.T) {
	payload := "{\"content\":[{\"type\":\"text\",\"text\":\"" + strings.Repeat("x", 4<<20) + "\"}]}"
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}
	if _, err := translateAnthropicResponse(response, nil); err == nil {
		t.Fatal("native Messages response above Prodex 4 MiB bound was accepted")
	}
}

func TestAnthropicReferenceStreamHasNoLifetimeTotalByteCap(t *testing.T) {
	state := anthropicStreamState{}
	padding := strings.Repeat("x", 900<<10)
	for index := 0; index < 10; index++ {
		event := []byte("{\"type\":\"ping\",\"padding\":\"" + padding + "\"}")
		if _, _, err := state.translate(event, time.Unix(123, 0)); err != nil {
			t.Fatalf("ping %d failed after cumulative stream bytes: %v", index, err)
		}
	}
	translated, supported, err := state.translate([]byte("{\"type\":\"message_start\",\"message\":{\"id\":\"msg_long\",\"model\":\"deepseek-chat\"}}"), time.Unix(123, 0))
	if err != nil || !supported || !strings.Contains(string(translated), "response.created") {
		t.Fatalf("post-ping message start = %q supported:%t err:%v", translated, supported, err)
	}
}

func TestAnthropicReferenceStreamAcceptsEventBelowFourMiB(t *testing.T) {
	text := strings.Repeat("x", 2<<20)
	upstream := strings.Join([]string{
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_large\",\"model\":\"deepseek-chat\"}}\n\n",
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + text + "\"}}\n\n",
		"data: {\"type\":\"message_stop\"}\n\n",
	}, "")
	body := deepSeekAnthropicSSE(io.NopCloser(strings.NewReader(upstream)), nil)
	defer body.Close()
	translated, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(translated), "response.output_text.delta") || !strings.Contains(string(translated), text[:1024]) || !strings.Contains(string(translated), "response.completed") {
		t.Fatalf("2 MiB native event was not translated; output bytes=%d", len(translated))
	}
}

func TestAnthropicReferenceStreamMalformedEventEmitsFailure(t *testing.T) {
	upstream := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_partial\"}}\n\n" +
		"event: content_block_delta\ndata: {malformed}\n\n"
	body := deepSeekAnthropicSSE(io.NopCloser(strings.NewReader(upstream)), nil)
	defer body.Close()
	translated, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(translated)
	if !strings.Contains(text, "event: response.created") ||
		!strings.Contains(text, "event: response.failed") ||
		!strings.Contains(text, `"code":"provider_stream_error"`) ||
		strings.Contains(text, "event: response.completed") {
		t.Fatalf("malformed Anthropic stream = %s", text)
	}
}

func TestAnthropicReferenceSimpleStreamUsesDeepSeekRuntimeEventShape(t *testing.T) {
	state := anthropicStreamState{requestID: 7}
	created, supported, err := state.translate([]byte(`{"type":"message_start","message":{"id":"msg_test","model":"deepseek-chat","usage":{"input_tokens":2}}}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("created = %q supported:%t err:%v", created, supported, err)
	}
	createdEvents := decodeAnthropicReferenceEvents(t, created)
	if len(createdEvents) != 1 {
		t.Fatalf("created events = %#v", createdEvents)
	}
	if got := createdEvents[0]; got["type"] != "response.created" || got["sequence_number"] != float64(0) || got["created_at"] != float64(123) {
		t.Fatalf("created event = %#v", got)
	} else if response := got["response"].(map[string]any); len(response) != 1 || response["id"] != "msg_test" {
		t.Fatalf("created response = %#v", response)
	}

	delta, supported, err := state.translate([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("delta = %q supported:%t err:%v", delta, supported, err)
	}
	deltaEvents := decodeAnthropicReferenceEvents(t, delta)
	if len(deltaEvents) != 2 {
		t.Fatalf("delta events = %#v", deltaEvents)
	}
	added := deltaEvents[0]
	if added["type"] != "response.output_item.added" || added["sequence_number"] != float64(1) || added["response_id"] != "msg_test" {
		t.Fatalf("text added = %#v", added)
	}
	item := added["item"].(map[string]any)
	if item["id"] != "msg_deepseek_7" || item["type"] != "message" || item["role"] != "assistant" {
		t.Fatalf("text item = %#v", item)
	}
	textDelta := deltaEvents[1]
	if textDelta["type"] != "response.output_text.delta" || textDelta["sequence_number"] != float64(2) || textDelta["created_at"] != float64(123) || textDelta["response_id"] != "msg_test" || textDelta["delta"] != "hello" {
		t.Fatalf("text delta = %#v", textDelta)
	}
	if _, found := textDelta["output_index"]; found {
		t.Fatalf("text delta unexpectedly contains output_index: %#v", textDelta)
	}

	completed, supported, err := state.translate([]byte(`{"type":"message_stop"}`), time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("completed = %q supported:%t err:%v", completed, supported, err)
	}
	completedEvents := decodeAnthropicReferenceEvents(t, completed)
	if len(completedEvents) != 2 {
		t.Fatalf("completion events = %#v", completedEvents)
	}
	done := completedEvents[0]
	if done["type"] != "response.output_item.done" || done["sequence_number"] != float64(3) || done["response_id"] != "msg_test" || done["item"].(map[string]any)["id"] != "msg_deepseek_7" {
		t.Fatalf("text done = %#v", done)
	}
	final := completedEvents[1]
	if final["type"] != "response.completed" || final["sequence_number"] != float64(4) || final["created_at"] != float64(123) || final["response"].(map[string]any)["id"] != "msg_test" {
		t.Fatalf("completed event = %#v", final)
	}
}

func decodeAnthropicReferenceEvents(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	decoder := sse.NewDecoder(nativeMessagesMaxBytes)
	raw := decoder.Feed(data)
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		var value map[string]any
		if err := json.Unmarshal(item, &value); err != nil {
			t.Fatal(err)
		}
		result = append(result, value)
	}
	return result
}

func TestAnthropicReferenceStreamFailureRedactsSecretMaterial(t *testing.T) {
	bearer := strings.Join([]string{"fixture", "token", "123"}, "_")
	apiKey := strings.Join([]string{"api", "sentinel", "value"}, "-")
	message := "upstream failed: Authorization: Bearer " + bearer + " url=https://example.test?api_key=" + apiKey
	payload, err := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]any{"type": "overloaded_error", "message": message},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := anthropicStreamState{requestID: 9}
	translated, supported, err := state.translate(payload, time.Unix(123, 0))
	if err != nil || !supported {
		t.Fatalf("failure = %q supported:%t err:%v", translated, supported, err)
	}
	text := string(translated)
	if strings.Contains(text, bearer) || strings.Contains(text, apiKey) {
		t.Fatalf("failure leaked credential material: %s", text)
	}
	events := decodeAnthropicReferenceEvents(t, translated)
	if len(events) != 1 {
		t.Fatalf("redacted failure events = %#v", events)
	}
	response := events[0]["response"].(map[string]any)
	failure := response["error"].(map[string]any)
	if failure["code"] != "overloaded_error" || !strings.Contains(failure["message"].(string), "<redacted>") {
		t.Fatalf("redacted failure = %#v", failure)
	}
}
