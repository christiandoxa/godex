package deepseek

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04355DeepSeekPreviousResponseReplaysBufferedHistory(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatalf("upstream request = %s: %v", body, err)
		}
		requests = append(requests, value)
		writer.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = io.WriteString(writer, `{"id":"chatcmpl_1","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","reasoning_content":"Need package metadata.","content":"I will inspect it.","tool_calls":[{"id":"call_1","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"cat package.json\"}"}}]}}]}`)
			return
		}
		_, _ = io.WriteString(writer, `{"id":"chatcmpl_2","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","content":"prodex"}}]}`)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "openai_chat"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	first, err := transport.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"model":"deepseek-v4-pro","input":"read package metadata"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()

	second, err := transport.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"model":"deepseek-v4-pro","previous_response_id":"chatcmpl_1","input":[{"type":"function_call_output","call_id":"call_1","output":"{\"name\":\"prodex\"}"}]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, second.Body)
	_ = second.Body.Close()

	if len(requests) != 2 {
		t.Fatalf("upstream requests = %d, want 2", len(requests))
	}
	messages, _ := requests[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("replayed messages = %#v", messages)
	}
	user := messages[0].(map[string]any)
	assistant := messages[1].(map[string]any)
	tool := messages[2].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	if user["role"] != "user" || user["content"] != "read package metadata" ||
		assistant["role"] != "assistant" || assistant["reasoning_content"] != "Need package metadata." ||
		len(calls) != 1 || calls[0].(map[string]any)["id"] != "call_1" ||
		tool["role"] != "tool" || tool["tool_call_id"] != "call_1" {
		t.Fatalf("replayed messages = %#v", messages)
	}
}

func TestProdex04355DeepSeekUnboundPreviousResponseIsAccepted(t *testing.T) {
	translated, err := TranslateResponsesRequest(
		[]byte(`{"model":"deepseek-v4-pro","previous_response_id":"unknown","input":"hello"}`),
		RequestOptions{Model: "deepseek-v4-pro"},
	)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(translated.Body, &body); err != nil {
		t.Fatal(err)
	}
	messages, _ := body["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["content"] != "hello" {
		t.Fatalf("unbound continuation translation = %#v", body)
	}
}

func TestProdex04355DeepSeekToolOutputReplaysNewestHistoryByCallID(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		requests = append(requests, value)
		writer.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			_, _ = io.WriteString(writer, `{"id":"resp-old","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-shared","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"old\"}"}}]}}]}`)
		case 2:
			_, _ = io.WriteString(writer, `{"id":"resp-new","choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-shared","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"new\"}"}}]}}]}`)
		default:
			_, _ = io.WriteString(writer, `{"id":"resp-done","choices":[{"message":{"role":"assistant","content":"done"}}]}`)
		}
	}))
	defer server.Close()
	transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "openai_chat"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	for _, body := range []string{
		`{"input":"old prompt"}`,
		`{"input":"new prompt"}`,
		`{"input":[{"type":"function_call_output","call_id":"call-shared","output":"result"}]}`,
	} {
		requestBody := []byte(body)
		for i := range requestBody {
			if requestBody[i] == '\\' && i+1 < len(requestBody) && requestBody[i+1] == '"' {
				requestBody = append(requestBody[:i], requestBody[i+1:]...)
				i--
			}
		}
		response, err := transport.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/responses", Body: requestBody}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	messages, _ := requests[2]["messages"].([]any)
	if len(messages) != 3 || messages[0].(map[string]any)["content"] != "new prompt" || messages[2].(map[string]any)["tool_call_id"] != "call-shared" {
		t.Fatalf("call-id replay = %#v", messages)
	}
}

func TestProdex04355DeepSeekConversationScopesDoNotLeak(t *testing.T) {
	store := newDeepSeekConversationStore()
	left := store.scoped("tenant-a")
	right := store.scoped("tenant-b")
	left.insert("resp-1", []any{map[string]any{"role": "user", "content": "secret-a"}})
	if got := right.history("resp-1"); len(got) != 0 {
		t.Fatalf("cross-scope history leaked = %#v", got)
	}
}

func TestProdex04355DeepSeekConversationStoreEvictsOldestPerScope(t *testing.T) {
	store := newDeepSeekConversationStore().scoped("tenant-a")
	for index := 0; index <= deepSeekConversationsPerScope; index++ {
		store.insert(fmt.Sprintf("resp-%d", index), []any{map[string]any{"role": "user", "content": index}})
	}
	if got := store.history("resp-0"); len(got) != 0 {
		t.Fatalf("oldest conversation survived scope cap = %#v", got)
	}
	if got := store.history(fmt.Sprintf("resp-%d", deepSeekConversationsPerScope)); len(got) == 0 {
		t.Fatal("newest conversation was evicted")
	}
}

func TestProdex04355DeepSeekStreamStoresConversationForReplay(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		requests = append(requests, value)
		writer.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_stream\",\"model\":\"deepseek-v4-pro\",\"choices\":[{\"delta\":{\"reasoning_content\":\"Need package \"}}]}\n\n")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_stream\",\"choices\":[{\"delta\":{\"reasoning_content\":\"metadata.\",\"content\":\"I will inspect it.\"}}]}\n\n")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_stream\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_stream\",\"function\":{\"name\":\"shell\",\"arguments\":\"{\\\"cmd\\\":\"}}]}}]}\n\n")
			_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_stream\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"cat package.json\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
			_, _ = io.WriteString(writer, "data: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(writer, "data: {\"id\":\"chatcmpl_done\",\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}))
	defer server.Close()
	transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "openai_chat"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	firstBody, _ := json.Marshal(map[string]any{"model": "deepseek-v4-pro", "stream": true, "input": "read package metadata"})
	first, err := transport.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/responses", Body: firstBody}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := io.ReadAll(first.Body)
	_ = first.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stream), "response.completed") {
		t.Fatalf("first stream = %s", stream)
	}

	secondBody, _ := json.Marshal(map[string]any{
		"model": "deepseek-v4-pro", "previous_response_id": "chatcmpl_stream",
		"input": []any{map[string]any{"type": "function_call_output", "call_id": "call_stream", "output": "package"}},
	})
	second, err := transport.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/responses", Body: secondBody}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, second.Body)
	_ = second.Body.Close()
	if len(requests) != 2 {
		t.Fatalf("upstream requests = %d", len(requests))
	}
	messages, _ := requests[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("stream replay messages = %#v", messages)
	}
	assistant := messages[1].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	if assistant["reasoning_content"] != "Need package metadata." || len(calls) != 1 || calls[0].(map[string]any)["id"] != "call_stream" {
		t.Fatalf("stream replay assistant = %#v", assistant)
	}
}

func TestProdex04355DeepSeekChatSSEOmitsEmptyDeltaAndPreservesWhitespace(t *testing.T) {
	upstream := io.NopCloser(strings.NewReader(
		"data: {\"id\":\"chatcmpl_empty\",\"choices\":[{\"delta\":{\"reasoning_content\":\"\",\"content\":\"\"}}]}\n\n" +
			"data: {\"id\":\"chatcmpl_empty\",\"choices\":[{\"delta\":{\"reasoning_content\":\" \",\"content\":\" \"},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n\n",
	))
	body := deepSeekChatSSE(upstream)
	defer body.Close()
	translated, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(translated)
	if strings.Count(text, "event: response.output_text.delta") != 1 ||
		!strings.Contains(text, `"delta":" "`) || strings.Contains(text, `"delta":""`) {
		t.Fatalf("translated empty/space deltas = %q", text)
	}
}

func TestProdex04355DeepSeekNativeBufferedResponseStoresConversation(t *testing.T) {
	testDeepSeekNativeConversationReplay(t, false)
}

func TestProdex04355DeepSeekNativeStreamStoresConversation(t *testing.T) {
	testDeepSeekNativeConversationReplay(t, true)
}

func testDeepSeekNativeConversationReplay(t *testing.T, stream bool) {
	t.Helper()
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatalf("native upstream request = %s: %v", body, err)
		}
		requests = append(requests, value)
		if len(requests) == 1 && stream {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(writer, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_native\",\"model\":\"deepseek-v4-pro\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
			_, _ = io.WriteString(writer, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
			_, _ = io.WriteString(writer, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"checking\"}}\n\n")
			_, _ = io.WriteString(writer, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_native\",\"name\":\"read_file\",\"input\":{}}}\n\n")
			_, _ = io.WriteString(writer, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"README.md\\\"}\"}}\n\n")
			_, _ = io.WriteString(writer, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":2}}\n\n")
			_, _ = io.WriteString(writer, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			_, _ = io.WriteString(writer, `{"id":"msg_native","model":"deepseek-v4-pro","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"call_native","name":"read_file","input":{"path":"README.md"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":2}}`)
			return
		}
		_, _ = io.WriteString(writer, `{"id":"msg_done","model":"deepseek-v4-pro","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{
		WebSearchMode: "anthropic", BetaBaseURL: server.URL,
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	firstBody, _ := json.Marshal(map[string]any{
		"model": "deepseek-v4-pro", "stream": stream, "input": "inspect readme", "web_search_options": map[string]any{},
	})
	first, err := transport.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/responses", Body: firstBody}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	secondBody, _ := json.Marshal(map[string]any{
		"model": "deepseek-v4-pro", "previous_response_id": "msg_native", "web_search_options": map[string]any{},
		"input": []any{map[string]any{"type": "function_call_output", "call_id": "call_native", "output": "file contents"}},
	})
	second, err := transport.Execute(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/responses", Body: secondBody}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, second.Body)
	_ = second.Body.Close()
	if len(requests) != 2 {
		t.Fatalf("native upstream requests = %d", len(requests))
	}
	messages, _ := requests[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("native replay messages = %#v", messages)
	}
	assistant := messages[1].(map[string]any)
	assistantBlocks, _ := assistant["content"].([]any)
	toolResult := messages[2].(map[string]any)
	toolResultBlocks, _ := toolResult["content"].([]any)
	if assistant["role"] != "assistant" || len(assistantBlocks) != 2 ||
		assistantBlocks[1].(map[string]any)["type"] != "tool_use" ||
		assistantBlocks[1].(map[string]any)["id"] != "call_native" ||
		toolResult["role"] != "user" || len(toolResultBlocks) != 1 ||
		toolResultBlocks[0].(map[string]any)["type"] != "tool_result" {
		t.Fatalf("native replay messages = %#v", messages)
	}
}
