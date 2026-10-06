package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestDeepSeekAutoWebSearchUsesNativeMessagesAndMapsJSON(t *testing.T) {
	type request struct {
		path, query, apiKey, authorization, version, userAgent string
		body                                                   map[string]any
	}
	var mu sync.Mutex
	var got request
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, input *http.Request) {
		body, _ := io.ReadAll(input.Body)
		mu.Lock()
		got.path, got.query = input.URL.Path, input.URL.RawQuery
		got.apiKey, got.authorization = input.Header.Get("x-api-key"), input.Header.Get("Authorization")
		got.version, got.userAgent = input.Header.Get("anthropic-version"), input.Header.Get("User-Agent")
		_ = json.Unmarshal(body, &got.body)
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"msg_search","model":"deepseek-v4-pro","content":[{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{"query":"release"}},{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[{"url":"https://example.test/release","title":"Release"}]},{"type":"text","text":"Found it."}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2,"server_tool_use":{"web_search_requests":1}}}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL+"/v1", "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses", RawQuery: "trace=one",
		Header: http.Header{"User-Agent": []string{"fixture-client"}},
		Body:   []byte(`{"model":"deepseek-v4-pro","input":"find release","tools":[{"type":"web_search_preview","search_context_size":"high","allowed_domains":["example.test"],"max_uses":2}],"metadata":{"request":"fixture"}}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("native buffered Content-Type = %q", got)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var translated map[string]any
	if err := json.Unmarshal(body, &translated); err != nil {
		t.Fatalf("translated response %q: %v", body, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got.path != "/anthropic/v1/messages" || got.query != "trace=one" || got.apiKey != "fixture-key" || got.authorization != "" || got.version != anthropicVersion || got.userAgent != "fixture-client" {
		t.Fatalf("native request headers/path = %#v", got)
	}
	if got.body["model"] != "deepseek-v4-pro" || got.body["max_tokens"] != float64(4096) || got.body["system"] != nil {
		t.Fatalf("native request = %#v", got.body)
	}
	messages := got.body["messages"].([]any)
	if messages[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"] != "find release" {
		t.Fatalf("native messages = %#v", messages)
	}
	tools := got.body["tools"].([]any)
	search := tools[0].(map[string]any)
	if search["type"] != "web_search_20250305" || search["name"] != "web_search" || search["allowed_domains"].([]any)[0] != "example.test" || search["max_uses"] != float64(2) || search["search_context_size"] != nil {
		t.Fatalf("native search tool = %#v", search)
	}
	output := translated["output"].([]any)
	searchCall := output[0].(map[string]any)
	if searchCall["type"] != "web_search_call" || searchCall["status"] != "completed" || searchCall["action"].(map[string]any)["sources"].([]any)[0].(map[string]any)["url"] != "https://example.test/release" {
		t.Fatalf("translated search result = %#v", searchCall)
	}
	if translated["tool_usage"].(map[string]any)["web_search"].(map[string]any)["num_requests"] != float64(1) || translated["metadata"].(map[string]any)["request"] != "fixture" {
		t.Fatalf("translated usage/metadata = %#v", translated)
	}
}

func TestDeepSeekAnthropicModeSelectsNativeMessages(t *testing.T) {
	var path, apiKey, authorization string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		apiKey = request.Header.Get("x-api-key")
		authorization = request.Header.Get("Authorization")
		_ = json.NewDecoder(request.Body).Decode(&body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"msg_anthropic","model":"deepseek-v4-pro","content":[]}`)
	}))
	defer server.Close()
	transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{
		WebSearchMode: "anthropic",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"model":"deepseek-v4-pro","input":"find it","web_search_options":{}}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if path != "/anthropic/v1/messages" || apiKey != "fixture-key" || authorization != "" {
		t.Fatalf("native Messages route/auth = %q / %q / %q", path, apiKey, authorization)
	}
	if body["model"] != "deepseek-v4-pro" || body["tools"] == nil {
		t.Fatalf("native Messages request = %#v", body)
	}
}

func TestDeepSeekNativeMessagesMapsInstructionsToolCallsAndChoice(t *testing.T) {
	translated, err := TranslateResponsesRequest([]byte(`{
		"instructions":"Be concise.",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"Read it"}]},
			{"type":"function_call","call_id":"call_test","name":"read_file","arguments":"{\"path\":\"/tmp/test\"}"},
			{"type":"function_call_output","call_id":"call_test","output":"contents"}
		],
		"tools":[{"type":"function","name":"read_file","description":"Read one file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}],
		"tool_choice":"required"
	}`), RequestOptions{Model: "deepseek-v4-pro"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := deepSeekAnthropicRequest(translated.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value["system"] != "Be concise." || value["tool_choice"].(map[string]any)["type"] != "any" {
		t.Fatalf("native system/tool_choice = %#v", value)
	}
	messages := value["messages"].([]any)
	if len(messages) != 3 || messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)["input"].(map[string]any)["path"] != "/tmp/test" {
		t.Fatalf("native messages = %#v", messages)
	}
	toolResult := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "call_test" || toolResult["content"] != "contents" {
		t.Fatalf("native tool result = %#v", toolResult)
	}
	tools := value["tools"].([]any)
	function := tools[0].(map[string]any)
	if function["name"] != "read_file" || function["description"] != "Read one file" || function["input_schema"].(map[string]any)["required"].([]any)[0] != "path" {
		t.Fatalf("native function tool = %#v", function)
	}
}

func TestDeepSeekAnthropicRequestNormalizesNamespacedToolCalls(t *testing.T) {
	body, err := deepSeekAnthropicRequest([]byte(`{
		"messages":[{"role":"assistant","tool_calls":[{"id":"call_test","function":{"name":"files.read_file","arguments":"{}"}}]}],
		"tools":[{"type":"function","name":"files.read_file","parameters":{"type":"object"}}],
		"tool_choice":{"type":"function","name":"files.read_file"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	tool := request["tools"].([]any)[0].(map[string]any)
	call := request["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	choice := request["tool_choice"].(map[string]any)
	for name, value := range map[string]any{
		"tool declaration": tool["name"], "tool call": call["name"], "tool choice": choice["name"],
	} {
		if value != "files--read_file" {
			t.Errorf("%s name = %#v", name, value)
		}
	}
}

func TestDeepSeekWebSearchModeFallbackPolicy(t *testing.T) {
	for _, test := range []struct {
		name          string
		mode          string
		wantPath      string
		wantSearch    bool
		wantLocalFail bool
	}{
		{name: "openai chat", mode: "openai_chat", wantPath: "/chat/completions", wantSearch: true},
		{name: "unset defaults to auto", mode: "", wantPath: "/chat/completions", wantSearch: false},
		{name: "auto", mode: "auto", wantPath: "/chat/completions", wantSearch: false},
		{name: "anthropic rejects unsafe fallback", mode: "anthropic", wantLocalFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var path string
			var body map[string]any
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				path = request.URL.Path
				content, _ := io.ReadAll(request.Body)
				_ = json.Unmarshal(content, &body)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
			}))
			defer server.Close()
			transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: test.mode}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			response, err := transport.Execute(context.Background(), proxymodel.Request{
				Method: http.MethodPost, Path: mountPath + "/responses",
				Body: []byte(`{"model":"deepseek-v4-pro","input":"find it","web_search_options":{},"response_format":{"type":"json_object"}}`),
			}, proxymodel.Account{})
			if test.wantLocalFail {
				var proxyError *proxymodel.Error
				if !errors.As(err, &proxyError) || proxyError.StatusCode != http.StatusBadRequest || calls != 0 {
					t.Fatalf("explicit Anthropic result/error/calls = %#v / %v / %d", response, err, calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if path != test.wantPath || calls != 1 {
				t.Fatalf("route/calls = %q / %d", path, calls)
			}
			_, hasSearch := body["web_search_options"]
			if hasSearch != test.wantSearch {
				t.Fatalf("forwarded body = %#v", body)
			}
		})
	}
}

func TestDeepSeekNativeMessagesRetriesOnlyBeforeFirstStreamEvent(t *testing.T) {
	t.Run("retry transient error as first event", func(t *testing.T) {
		var models []string
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			models = append(models, body["model"].(string))
			writer.Header().Set("Content-Type", "text/event-stream")
			flusher := writer.(http.Flusher)
			if len(models) == 1 {
				_, _ = io.WriteString(writer, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"retry\"}}\n\n")
				flusher.Flush()
				return
			}
			for _, chunk := range []string{
				"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_ok\",\"model\":\"deepseek-v4-flash\"}}\n\n",
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\"}}\n\n",
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ready\"}}\n\n",
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			} {
				_, _ = io.WriteString(writer, chunk[:len(chunk)/2])
				flusher.Flush()
				_, _ = io.WriteString(writer, chunk[len(chunk)/2:])
				flusher.Flush()
			}
		}))
		defer server.Close()
		transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "auto", StreamIdleTimeout: 250 * time.Millisecond}, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		defer transport.Close()
		response, err := transport.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: mountPath + "/responses",
			Body: []byte(`{"model":"pro","input":"search","web_search_options":{}}`),
		}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		stream, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(models) != 2 || models[0] != "deepseek-v4-pro" || models[1] != "deepseek-v4-flash" || !strings.Contains(string(stream), "event: response.created") || !strings.Contains(string(stream), `"delta":"ready"`) {
			t.Fatalf("models/stream = %#v / %s", models, stream)
		}
		if !response.FirstEventRetryUsed || !response.FirstEventCommitted || response.PrecommitFailure != nil {
			t.Fatalf("model fallback commitment = %#v", response)
		}
	})

	t.Run("commit after first non-error event", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls++
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_started\"}}\n\n")
			writer.(http.Flusher).Flush()
			_, _ = io.WriteString(writer, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\"}}\n\n")
		}))
		defer server.Close()
		transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "auto"}, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		defer transport.Close()
		response, err := transport.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: mountPath + "/responses",
			Body: []byte(`{"model":"pro","input":"search","web_search_options":{}}`),
		}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if !response.FirstEventCommitted || response.FirstEventRetryUsed || response.PrecommitFailure != nil {
			t.Fatalf("first non-error event commitment = %#v", response)
		}
		stream, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 || !strings.Contains(string(stream), "event: response.created") ||
			!strings.Contains(string(stream), "event: response.failed") || strings.Contains(string(stream), "event: error") {
			t.Fatalf("calls/stream = %d / %s", calls, stream)
		}
	})

	t.Run("premature EOF fails committed stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_eof\"}}\n\n")
		}))
		defer server.Close()
		transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "auto"}, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		defer transport.Close()
		response, err := transport.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: mountPath + "/responses",
			Body: []byte(`{"model":"pro","input":"search","web_search_options":{}}`),
		}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		stream, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(stream), "event: response.created") ||
			!strings.Contains(string(stream), `"code":"provider_stream_error"`) ||
			!strings.Contains(string(stream), "event: response.failed") ||
			strings.Contains(string(stream), "event: response.completed") {
			t.Fatalf("premature EOF stream = %s", stream)
		}
	})
}

func TestDeepSeekNativeMessagesReportsUnspentFirstEventFailures(t *testing.T) {
	for _, fixture := range []struct {
		name          string
		model         string
		retryUsed     bool
		event         string
		wantCode      string
		wantTransport bool
		wantUsed      bool
		wantCommitted bool
	}{
		{
			name: "retryable event can rotate credentials", model: "deepseek-v4-flash",
			event:    `{"type":"error","error":{"type":"overloaded_error"}}`,
			wantCode: "overloaded_error",
		},
		{
			name: "request budget prevents another model retry", model: "pro", retryUsed: true,
			event:    `{"type":"error","error":{"type":"overloaded_error"}}`,
			wantCode: "overloaded_error", wantUsed: true, wantCommitted: true,
		},
		{
			name: "error type takes precedence over retryable code", model: "pro",
			event:    `{"type":"error","error":{"type":"unknown","code":"overloaded_error"}}`,
			wantCode: "unknown", wantCommitted: true,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls++
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(writer, "event: error\ndata: "+fixture.event+"\n\n")
			}))
			defer server.Close()
			transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "auto"}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			response, err := transport.Execute(context.Background(), proxymodel.Request{
				Method: http.MethodPost, Path: mountPath + "/responses", FirstEventRetryUsed: fixture.retryUsed,
				Body: []byte(`{"model":"` + fixture.model + `","input":"search","web_search_options":{}}`),
			}, proxymodel.Account{})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if calls != 1 || response.FirstEventRetryUsed != fixture.wantUsed || response.FirstEventCommitted != fixture.wantCommitted {
				t.Fatalf("calls/retry/commit = %d / %#v", calls, response)
			}
			if response.PrecommitFailure == nil || response.PrecommitFailure.Code != fixture.wantCode || response.PrecommitFailure.Transport != fixture.wantTransport {
				t.Fatalf("precommit failure = %#v", response.PrecommitFailure)
			}
		})
	}
}

func TestDeepSeekNativeMessagesStreamCompletionIncludesOutputUsageAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`event: message_start` + "\ndata: " + `{"type":"message_start","message":{"id":"msg_stream","model":"deepseek-v4-pro","usage":{"input_tokens":5}}}` + "\n\n",
			`event: content_block_start` + "\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text"}}` + "\n\n",
			`event: content_block_delta` + "\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Found it."}}` + "\n\n",
			`event: content_block_start` + "\ndata: " + `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_read","name":"read_file","input":{}}}` + "\n\n",
			`event: content_block_delta` + "\ndata: " + `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"/tmp/a\"}"}}` + "\n\n",
			`event: content_block_start` + "\ndata: " + `{"type":"content_block_start","index":2,"content_block":{"type":"server_tool_use","id":"srv_search","name":"web_search","input":{}}}` + "\n\n",
			`event: content_block_delta` + "\ndata: " + `{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"release\"}"}}` + "\n\n",
			`event: content_block_start` + "\ndata: " + `{"type":"content_block_start","index":3,"content_block":{"type":"web_search_tool_result","tool_use_id":"srv_search","content":[{"type":"web_search_result","url":"https://example.test/release","title":"Release"}]}}` + "\n\n",
			`event: message_delta` + "\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":3,"server_tool_use":{"web_search_requests":1}}}` + "\n\n",
			`event: message_stop` + "\ndata: " + `{"type":"message_stop"}` + "\n\n",
		} {
			_, _ = io.WriteString(writer, event)
			writer.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{
		WebSearchMode: "auto", SSELookaheadTimeout: time.Second,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		RequestID: 42, Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"model":"pro","input":"find release","web_search_options":{},"metadata":{"request":"stream-fixture"}}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("native stream Content-Type = %q", got)
	}
	stream, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoder := sse.NewDecoder(streamEventMaxBytes)
	var completed map[string]any
	for _, data := range decoder.Feed(stream) {
		var event map[string]any
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event["type"] == "response.completed" {
			completed, _ = event["response"].(map[string]any)
		}
	}
	if completed == nil {
		t.Fatalf("completed response missing from SSE stream: %s", stream)
	}
	if !strings.Contains(string(stream), "event: response.output_item.done") ||
		!strings.Contains(string(stream), `"call_id":"call_read","delta"`) ||
		!strings.Contains(string(stream), `"id":"msg_deepseek_42"`) {
		t.Fatalf("stream omitted output completion or tool call ID: %s", stream)
	}
	output := completed["output"].([]any)
	if len(output) != 3 {
		t.Fatalf("completed output = %#v", output)
	}
	message := output[0].(map[string]any)
	content := message["content"].([]any)
	if content[0].(map[string]any)["text"] != "Found it." {
		t.Fatalf("completed text = %#v", content)
	}
	call := output[1].(map[string]any)
	if call["type"] != "function_call" || call["call_id"] != "call_read" || !strings.Contains(call["arguments"].(string), "/tmp/a") {
		t.Fatalf("completed function call = %#v", call)
	}
	search := output[2].(map[string]any)
	if search["type"] != "web_search_call" || search["status"] != "completed" ||
		search["action"].(map[string]any)["queries"].([]any)[0] != "release" ||
		search["action"].(map[string]any)["sources"].([]any)[0].(map[string]any)["url"] != "https://example.test/release" {
		t.Fatalf("completed web search = %#v", search)
	}
	usage := completed["usage"].(map[string]any)
	if usage["input_tokens"] != float64(5) || usage["output_tokens"] != float64(3) || usage["total_tokens"] != float64(8) {
		t.Fatalf("completed usage = %#v", usage)
	}
	if completed["tool_usage"].(map[string]any)["web_search"].(map[string]any)["num_requests"] != float64(1) ||
		completed["metadata"].(map[string]any)["request"] != "stream-fixture" ||
		completed["metadata"].(map[string]any)["anthropic"].(map[string]any)["stop_reason"] != "tool_use" {
		t.Fatalf("completed metadata = %#v", completed["metadata"])
	}
}
