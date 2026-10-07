package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04357GeminiResponsesUsesNativeGenerateContentWire(t *testing.T) {
	var capturedPath, capturedAPIKey, capturedAuthorization string
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		capturedPath = request.URL.Path
		capturedAPIKey = request.Header.Get("x-goog-api-key")
		capturedAuthorization = request.Header.Get("Authorization")
		if err := json.NewDecoder(request.Body).Decode(&capturedBody); err != nil {
			t.Errorf("decode Gemini native request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"responseId":"resp-native","modelVersion":"gemini-3.8-flash","candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}`)
	}))
	defer server.Close()

	transport, err := NewRuntimeTransport(server.URL+"/v1beta", "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost,
		Path:   mountPath + "/responses",
		Body:   []byte(`{"model":"gemini-3.8-flash","input":"hello"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if capturedPath != "/v1beta/models/gemini-3.8-flash:generateContent" ||
		capturedAPIKey != "fixture-key" || capturedAuthorization != "" {
		t.Fatalf("Gemini upstream = path:%q api-key:%q auth:%q", capturedPath, capturedAPIKey, capturedAuthorization)
	}
	contents, _ := capturedBody["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("Gemini native request body = %#v", capturedBody)
	}
	first := contents[0].(map[string]any)
	if first["role"] != "user" ||
		first["parts"].([]any)[0].(map[string]any)["text"] != "hello" {
		t.Fatalf("Gemini contents = %#v", contents)
	}
	var translated map[string]any
	if err := json.Unmarshal(body, &translated); err != nil {
		t.Fatal(err)
	}
	if translated["id"] != "resp-native" || translated["model"] != "gemini-3.8-flash" {
		t.Fatalf("Gemini translated identity = %#v", translated)
	}
	output := translated["output"].([]any)
	message := output[0].(map[string]any)
	if message["type"] != "message" ||
		message["content"].([]any)[0].(map[string]any)["text"] != "ok" {
		t.Fatalf("Gemini translated output = %#v", output)
	}
	usage := translated["usage"].(map[string]any)
	if usage["input_tokens"] != float64(3) || usage["output_tokens"] != float64(2) || usage["total_tokens"] != float64(5) {
		t.Fatalf("Gemini usage = %#v", usage)
	}
}

func TestProdex04357GeminiGroundingPreservesAliasDedupeQueriesAndCitationOrder(t *testing.T) {
	native := `{
	  "responseId":"resp_grounded",
	  "modelVersion":"gemini-test",
	  "candidates":[{
	    "content":{"parts":[{"text":"visible"}]},
	    "finishReason":"STOP",
	    "groundingMetadata":{
	      "webSearchQueries":[7,"query",""],
	      "groundingChunks":[
	        {"web":{"uri":"   ","url":"https://must-not-fallback.example"}},
	        {"retrievedContext":{"retrievedUrl":"https://dup.example","title":"First","urlRetrievalStatus":"OK"}},
	        {"web":{"url":"https://dup.example","title":"Later"}}
	      ]
	    },
	    "citationMetadata":{
	      "citationSources":[{"retrieved_url":"https://citation.example","title":" ","url_retrieval_status":{"state":"ready"}}],
	      "citations":[{"title":"Source","uri":"https://example.com/source"}]
	    },
	    "urlContextMetadata":{"url_metadata":[{"url":"https://url.example","status":0}]}
	  }]
	}`
	response, err := translateResponse(&http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(native)),
	}, nil)
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
	output, _ := value["output"].([]any)
	if len(output) < 3 {
		t.Fatalf("Gemini grounded output too short: %s", body)
	}
	if output[0].(map[string]any)["type"] != "message" ||
		output[1].(map[string]any)["type"] != "web_search_call" ||
		output[2].(map[string]any)["type"] != "message" {
		t.Fatalf("Gemini grounded output order = %#v", output)
	}
	action := output[1].(map[string]any)["action"].(map[string]any)
	queries, _ := action["queries"].([]any)
	if len(queries) != 2 || queries[0] != "query" || queries[1] != "" {
		t.Fatalf("Gemini grounding queries = %#v", queries)
	}
	sources, _ := action["sources"].([]any)
	if len(sources) != 3 ||
		sources[0].(map[string]any)["url"] != "https://dup.example" ||
		sources[1].(map[string]any)["url"] != "https://example.com/source" ||
		sources[2].(map[string]any)["url"] != "https://url.example" {
		t.Fatalf("Gemini grounding sources = %#v", sources)
	}
	citation := output[2].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if citation != "Citations:\n(Source) https://example.com/source" {
		t.Fatalf("Gemini citation text = %#v", citation)
	}
}

func TestProdex04357GeminiGroundingCitationSourcesAliasIsPreserved(t *testing.T) {
	root := map[string]any{
		"responseId": "resp_alias",
		"candidates": []any{map[string]any{
			"finishReason": "STOP",
			"citationMetadata": map[string]any{
				"citationSources": []any{map[string]any{
					"retrieved_url":        "https://citation.example",
					"title":                " ",
					"url_retrieval_status": map[string]any{"state": "ready"},
				}},
			},
		}},
	}
	value := geminiNativeResponsesValue(root, nil, 0)
	output := value["output"].([]any)
	if len(output) != 1 || output[0].(map[string]any)["type"] != "web_search_call" {
		t.Fatalf("citationSources output = %#v", output)
	}
	sources := output[0].(map[string]any)["action"].(map[string]any)["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("citationSources sources = %#v", sources)
	}
	source := sources[0].(map[string]any)
	if source["url"] != "https://citation.example" || source["title"] != nil {
		t.Fatalf("citationSources source = %#v", source)
	}
	status := source["status"].(map[string]any)
	if status["state"] != "ready" {
		t.Fatalf("citationSources status = %#v", status)
	}
}

func TestProdex04357GeminiBufferedSpecialToolItemsAndNamespaceMapping(t *testing.T) {
	root := map[string]any{
		"responseId": "resp_tools",
		"candidates": []any{map[string]any{
			"finishReason": "STOP",
			"content": map[string]any{"parts": []any{
				map[string]any{"functionCall": map[string]any{
					"id": "call-search", "name": "tool_search",
					"args": map[string]any{"query": "repo"},
				}},
				map[string]any{"functionCall": map[string]any{
					"id": "call-patch", "name": "apply_patch",
					"args": map[string]any{"path": "a.txt", "old_string": "", "new_string": "hello"},
				}},
				map[string]any{
					"thoughtSignature": "sig",
					"functionCall": map[string]any{
						"id": "call-shell", "name": "ns--shell",
						"args": map[string]any{"cmd": "git status"},
					},
				},
			}},
		}},
	}
	value := geminiNativeResponsesValue(root, nil, 0)
	output := value["output"].([]any)
	if len(output) != 3 {
		t.Fatalf("special tool output = %#v", output)
	}
	search := output[0].(map[string]any)
	if search["type"] != "tool_search_call" || search["call_id"] != "call-search" ||
		search["execution"] != "client" || search["arguments"].(map[string]any)["query"] != "repo" {
		t.Fatalf("tool_search item = %#v", search)
	}
	patch := output[1].(map[string]any)
	wantPatch := "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch"
	if patch["type"] != "custom_tool_call" || patch["name"] != "apply_patch" ||
		patch["input"] != wantPatch {
		t.Fatalf("apply_patch item = %#v", patch)
	}
	shell := output[2].(map[string]any)
	if shell["type"] != "function_call" || shell["namespace"] != "ns" || shell["name"] != "shell" ||
		shell["gemini_thought_signature"] != "sig" ||
		!strings.Contains(shell["arguments"].(string), "rtk git status") {
		t.Fatalf("namespaced shell item = %#v", shell)
	}
}

func TestProdex04357GeminiBufferedUsageDefaultsAndMaxTokensDetails(t *testing.T) {
	value := geminiNativeResponsesValue(map[string]any{
		"responseId":     "resp_incomplete",
		"usageMetadata":  "wrong-type",
		"promptFeedback": []any{"wrong-type"},
		"candidates": []any{map[string]any{
			"finishReason": "MAX_TOKENS",
			"content":      map[string]any{"parts": []any{map[string]any{"text": "partial"}}},
		}},
	}, nil, 0)
	if value["status"] != "incomplete" {
		t.Fatalf("MAX_TOKENS status = %#v", value)
	}
	details := value["incomplete_details"].(map[string]any)
	if details["reason"] != "max_output_tokens" ||
		details["message"] != "Gemini stopped because it reached the maximum output token limit." {
		t.Fatalf("MAX_TOKENS details = %#v", details)
	}
	usage := value["usage"].(map[string]any)
	if usage["input_tokens"] != uint64(0) || usage["output_tokens"] != uint64(0) ||
		usage["total_tokens"] != uint64(0) {
		t.Fatalf("wrong-type usage defaults = %#v", usage)
	}
	gemini := value["metadata"].(map[string]any)["gemini"].(map[string]any)
	if gemini["usageMetadata"] != "wrong-type" {
		t.Fatalf("raw wrong-type usage metadata was not preserved: %#v", gemini)
	}
}
