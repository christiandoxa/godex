package deepseek

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestDeepSeekRouteSurfaceMatchesRegistry(t *testing.T) {
	accepted := []string{
		mountPath + "/responses",
		mountPath + "/v1/responses",
		mountPath + "/responses/compact",
		mountPath + "/chat/completions",
		mountPath + "/messages",
		mountPath + "/models",
		mountPath + "/models/deepseek-v4-pro",
	}
	for _, path := range accepted {
		if _, err := runtimeRoute(path); err != nil {
			t.Fatalf("route %q rejected: %v", path, err)
		}
	}
	for _, path := range []string{
		mountPath + "/v2/responses",
		mountPath + "/responses/",
		mountPath + "/v1//responses",
		mountPath + "/v1/%2e%2e/responses",
		mountPath + "/embeddings",
	} {
		if _, err := runtimeRoute(path); err == nil {
			t.Fatalf("unsupported route %q accepted", path)
		}
	}
}

func TestDeepSeekMessagesPathMatchesReferenceNormalization(t *testing.T) {
	for base, want := range map[string]string{
		"":              "/anthropic/v1/messages",
		"/v1":           "/anthropic/v1/messages",
		"/beta":         "/anthropic/v1/messages",
		"/anthropic":    "/anthropic/v1/messages",
		"/anthropic/v1": "/anthropic/v1/messages",
		"/tenant/v1":    "/tenant/anthropic/v1/messages",
	} {
		if got := deepSeekMessagesPath(base); got != want {
			t.Fatalf("messages path for %q = %q, want %q", base, got, want)
		}
	}
}

func TestDeepSeekResponsesTranslateFallbackAndHeaders(t *testing.T) {
	type captured struct {
		path  string
		auth  string
		model string
	}
	var requests []captured
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		model, _ := value["model"].(string)
		requests = append(requests, captured{path: request.URL.Path, auth: request.Header.Get("Authorization"), model: model})
		writer.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":{"code":"model_not_supported"}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"chat_ok","model":"deepseek-v4-flash","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`))
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"model":"pro","input":"hello"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"object":"response"`) {
		t.Fatalf("translated response = status:%d body:%s", response.StatusCode, body)
	}
	if len(requests) != 2 || requests[0].model != "deepseek-v4-pro" || requests[1].model != "deepseek-v4-flash" {
		t.Fatalf("fallback requests = %#v", requests)
	}
	for _, request := range requests {
		if request.path != "/chat/completions" || request.auth != "Bearer fixture-key" {
			t.Fatalf("request = %#v", request)
		}
	}
}

func TestDeepSeekStrictResponsesUseConfiguredBetaBase(t *testing.T) {
	type captured struct {
		path, query, searchContext string
	}
	requests := make(chan captured, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		search, _ := value["web_search_options"].(map[string]any)
		contextSize, _ := search["search_context_size"].(string)
		requests <- captured{path: request.URL.Path, query: request.URL.RawQuery, searchContext: contextSize}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	transport, err := NewRuntimeTransportWithOptions(server.URL+"/v1", "fixture-key", RequestOptions{
		StrictTools: true, WebSearchMode: "openai_chat", BetaBaseURL: server.URL + "/tenant/beta/",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses", RawQuery: "trace=one",
		Body: []byte(`{"input":"search","web_search_options":{"search_context_size":"high"}}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if got := <-requests; got.path != "/tenant/beta/chat/completions" || got.query != "trace=one" || got.searchContext != "high" {
		t.Fatalf("DeepSeek beta request = %#v", got)
	}
}

func TestDeepSeekBetaBaseURLDefaultsAndRejectsMalformedValues(t *testing.T) {
	transport, err := NewRuntimeTransportWithOptions("https://api.deepseek.com", "fixture-key", RequestOptions{StrictTools: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := transport.betaUpstream.String(); got != defaultBetaBaseURL {
		t.Fatalf("default beta base URL = %q", got)
	}
	if got := transport.target(route{kind: routeResponses}, "trace=one"); got != defaultBetaBaseURL+"/chat/completions?trace=one" {
		t.Fatalf("default beta target = %q", got)
	}
	transport.Close()

	for _, baseURL := range []string{
		"file:///tmp/deepseek", "https://user:pass@example.test/beta", "https://example.test/beta?key=value",
		"https://example.test/beta#fragment", "https://bad host/beta", "https://example.test/beta path",
	} {
		if _, err := NewRuntimeTransportWithOptions("https://api.deepseek.com", "fixture-key", RequestOptions{BetaBaseURL: baseURL}, nil); err == nil {
			t.Fatalf("malformed beta base URL %q accepted", baseURL)
		}
	}
}

func TestDeepSeekOffModeRejectsSearchBeforeUpstream(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	transport, err := NewRuntimeTransportWithOptions(server.URL, "fixture-key", RequestOptions{WebSearchMode: "off"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	_, err = transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(`{"input":"search","web_search_options":{}}`),
	}, proxymodel.Account{})
	var proxyError *proxymodel.Error
	if !errors.As(err, &proxyError) || proxyError.StatusCode != http.StatusBadRequest || !strings.Contains(err.Error(), "web search mode is off") {
		t.Fatalf("off-mode response error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("off-mode search reached upstream %d time(s)", calls)
	}
}

func TestDeepSeekAdvancedResponsesTranslateBeforeUpstream(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		content, _ := io.ReadAll(request.Body)
		var body map[string]any
		if err := json.Unmarshal(content, &body); err != nil {
			t.Fatalf("upstream body = %s, err = %v", content, err)
		}
		bodies = append(bodies, body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chat_ok","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	for _, body := range []string{
		`{"input":"hello","reasoning":{"effort":"high"}}`,
		`{"input":"hello","response_format":{"type":"json_object"}}`,
	} {
		response, err := transport.Execute(context.Background(), proxymodel.Request{
			Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(body),
		}, proxymodel.Account{})
		if err != nil {
			t.Fatalf("advanced request %s error = %v", body, err)
		}
		response.Body.Close()
	}
	if len(bodies) != 2 {
		t.Fatalf("advanced requests reaching upstream = %d", len(bodies))
	}
	if bodies[0]["reasoning_effort"] != "high" {
		t.Fatalf("reasoning request = %#v", bodies[0])
	}
	thinking, ok := bodies[0]["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking request = %#v", bodies[0])
	}
	if format, ok := bodies[1]["response_format"].(map[string]any); !ok || format["type"] != "json_object" {
		t.Fatalf("JSON response format = %#v", bodies[1])
	}
	messages := bodies[1]["messages"].([]any)
	if len(messages) < 2 || messages[0].(map[string]any)["content"] != "Respond with valid JSON only." {
		t.Fatalf("JSON prompt messages = %#v", messages)
	}

}

func TestDeepSeekMalformedWebSearchFailsBeforeUpstream(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	_, err = transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses",
		Body: []byte(`{"input":"hello","web_search_options":{"search_context_size":"huge"}}`),
	}, proxymodel.Account{})
	if err == nil || !strings.Contains(err.Error(), "search_context_size") {
		t.Fatalf("malformed web-search request error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("malformed web-search request reached upstream %d time(s)", calls)
	}
}

func TestDeepSeekChatAndMessagesRemainPassthrough(t *testing.T) {
	type captured struct {
		path, authorization, apiKey, version, body string
	}
	requests := make(chan captured, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- captured{
			path: request.URL.Path, authorization: request.Header.Get("Authorization"),
			apiKey: request.Header.Get("x-api-key"), version: request.Header.Get("anthropic-version"), body: string(body),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransportWithOptions(server.URL+"/v1", "fixture-key", RequestOptions{
		StrictTools: true, BetaBaseURL: server.URL + "/beta",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	chatBody := `{"model":"flash","messages":[{"role":"user","content":"chat"}]}`
	messagesBody := `{"model":"deepseek-chat","messages":[{"role":"user","content":"native"}]}`
	for path, body := range map[string]string{
		mountPath + "/chat/completions": chatBody,
		mountPath + "/messages":         messagesBody,
	} {
		response, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: path, Body: []byte(body)}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	for index := 0; index < 2; index++ {
		got := <-requests
		switch got.path {
		case "/v1/chat/completions":
			if got.authorization != "Bearer fixture-key" || got.apiKey != "" || got.version != "" || got.body != chatBody {
				t.Fatalf("chat passthrough = %#v", got)
			}
		case "/anthropic/v1/messages":
			if got.authorization != "" || got.apiKey != "fixture-key" || got.version != anthropicVersion || got.body != messagesBody {
				t.Fatalf("messages passthrough = %#v", got)
			}
		default:
			t.Fatalf("unexpected passthrough request = %#v", got)
		}
	}
}

func TestDeepSeekModelsAndCompactAreLocal(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	models, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: mountPath + "/models"}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	modelsBody, _ := io.ReadAll(models.Body)
	models.Body.Close()
	for _, model := range []string{"deepseek-v4-pro", "deepseek-v4-flash", "deepseek-chat", "deepseek-reasoner"} {
		if !strings.Contains(string(modelsBody), model) {
			t.Fatalf("models body missing %q: %s", model, modelsBody)
		}
	}
	compact, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses/compact",
		Body: []byte(`{"model":"deepseek-v4-pro","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"retain me"}]}]}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	compactBody, _ := io.ReadAll(compact.Body)
	compact.Body.Close()
	if called || compact.Header.Get("X-Godex-Compact-Provider") != "deepseek" || !strings.Contains(string(compactBody), "retain me") {
		t.Fatalf("local endpoints = called:%t headers:%v body:%s", called, compact.Header, compactBody)
	}
}

func TestDeepSeekBare429DoesNotAdvanceModel(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"too many requests"}}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(`{"model":"pro","input":"hello"}`)}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if calls != 1 || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("calls/status = %d / %d", calls, response.StatusCode)
	}
}

func TestDeepSeek529RequiresAProviderErrorSignalForModelFallback(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     int
		body       string
		wantCalls  int
		wantStatus int
	}{
		{name: "bare 529", status: 529, body: `{}`, wantCalls: 1, wantStatus: 529},
		{name: "529 with overload text", status: 529, body: `{"error":{"message":"server overloaded"}}`, wantCalls: 2, wantStatus: http.StatusOK},
		{name: "503 status signal", status: http.StatusServiceUnavailable, body: `{}`, wantCalls: 2, wantStatus: http.StatusOK},
		{name: "structured 429 code", status: http.StatusTooManyRequests, body: `{"error":{"code":"rate_limit_exceeded"}}`, wantCalls: 2, wantStatus: http.StatusOK},
		{name: "reason code", status: http.StatusBadRequest, body: `{"error":{"reason":"rate_limit_error"}}`, wantCalls: 2, wantStatus: http.StatusOK},
		{name: "unsupported shared quota code", status: http.StatusBadRequest, body: `{"error":{"code":"usage_limit_reached"}}`, wantCalls: 1, wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls++
				writer.Header().Set("Content-Type", "application/json")
				if calls == 1 {
					writer.WriteHeader(test.status)
					_, _ = io.WriteString(writer, test.body)
					return
				}
				_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"ok"}}]}`)
			}))
			defer server.Close()
			transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			response, err := transport.Execute(context.Background(), proxymodel.Request{
				Method: http.MethodPost, Path: mountPath + "/responses",
				Body: []byte(`{"model":"pro","input":"hello"}`),
			}, proxymodel.Account{})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if calls != test.wantCalls || response.StatusCode != test.wantStatus {
				t.Fatalf("calls/status = %d/%d, want %d/%d", calls, response.StatusCode, test.wantCalls, test.wantStatus)
			}
			if test.wantCalls == 1 && string(body) != test.body {
				t.Fatalf("error body = %q, want exact upstream body %q", body, test.body)
			}
		})
	}
}

func TestDeepSeekBufferedResponseMergesRequestAndProviderMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"deepseek-v4-pro",
			"choices":[{"message":{"content":"done","reasoning_content":"thought"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"prompt_cache_hit_tokens":5,"prompt_cache_miss_tokens":6}
		}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body: []byte(`{
			"input":"hello",
			"metadata":{"tag":"one","deepseek":{"existing":"keep"}},
			"client_metadata":{"client":true},
			"prompt_cache_key":"cache-key",
			"response_format":{"type":"json_schema"}
		}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	metadata := value["metadata"].(map[string]any)
	if metadata["tag"] != "one" || metadata["client_metadata"].(map[string]any)["client"] != true || metadata["prompt_cache_key"] != "cache-key" {
		t.Fatalf("response metadata = %#v", metadata)
	}
	deepseek := metadata["deepseek"].(map[string]any)
	if deepseek["existing"] != "keep" || deepseek["reasoning_content"] != "thought" || deepseek["finish_reason"] != "stop" {
		t.Fatalf("provider metadata = %#v", deepseek)
	}
	if deepseek["degraded_response_format"].(map[string]any)["from"] != "json_schema" {
		t.Fatalf("degraded response metadata = %#v", deepseek)
	}
	usage := value["usage"].(map[string]any)
	if usage["input_tokens_details"].(map[string]any)["cached_tokens"] != float64(5) {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestDeepSeekResponsesTranslateStreamWithProviderShaping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer,
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_shell\",\"function\":{\"name\":\"functions.exec_command\",\"arguments\":\"{\\\"cmd\\\":\\\"ls\\\"}\"}}]}}]}\n\n"+
				"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking\"}}]}\n\n"+
				"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"+
				"data: [DONE]\n\n",
		)
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"input":"hello","stream":true,"stream_options":{"include_usage":true}}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.function_call_arguments.delta",
		`"call_id":"call_shell"`,
		`"delta":"{\"cmd\":\"ls\"}"`,
		"event: response.reasoning_summary_text.delta",
		`"delta":"thinking"`,
		"event: response.output_item.done",
		`"arguments":"{\"cmd\":\"rtk ls\"}"`,
		"event: response.completed",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("translated DeepSeek stream missing %q: %s", want, body)
		}
	}
	if response.StatusCode != http.StatusOK || strings.Contains(text, `"delta":""`) {
		t.Fatalf("translated DeepSeek stream = status:%d body:%s", response.StatusCode, body)
	}
}

func TestDeepSeekBufferedResponseUsesTaggedSparseDefaults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"lookup","arguments":"{}"}}]}}]}`))
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"input":"hello"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	assertDeepSeekUUIDv7(t, value["id"], "resp_deepseek_")
	if value["model"] != "deepseek-chat" {
		t.Fatalf("sparse DeepSeek response defaults = %#v", value)
	}
	tool := value["output"].([]any)[0].(map[string]any)
	assertDeepSeekUUIDv7(t, tool["call_id"], "call_deepseek_")
}

func TestDeepSeekMetadataMergeMatchesProdexShallowObjectPolicy(t *testing.T) {
	response := map[string]any{"metadata": map[string]any{
		"scalar": false,
		"object": map[string]any{"old": float64(1), "nested": map[string]any{"a": float64(1)}},
	}}
	mergeResponseMetadata(response, map[string]any{
		"scalar":  map[string]any{"new": float64(2)},
		"object":  map[string]any{"new": float64(2), "nested": map[string]any{"b": float64(2)}},
		"missing": map[string]any{},
	})
	metadata := response["metadata"].(map[string]any)
	if metadata["scalar"] != false {
		t.Fatalf("scalar metadata was overwritten: %#v", metadata)
	}
	object := metadata["object"].(map[string]any)
	if object["old"] != float64(1) || object["new"] != float64(2) {
		t.Fatalf("object metadata = %#v", object)
	}
	nested := object["nested"].(map[string]any)
	if len(nested) != 1 || nested["b"] != float64(2) {
		t.Fatalf("nested metadata = %#v", nested)
	}
}
