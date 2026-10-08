package deepseek

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeepSeekResponsesRequestMapsAdvancedControls(t *testing.T) {
	translated, err := ResponsesRequest([]byte(`{
		"model":"deepseek-chat",
		"instructions":"system rules",
		"input":"hello",
		"stream":true,
		"temperature":0.2,
		"top_p":0.8,
		"max_output_tokens":512,
		"logprobs":true,
		"top_logprobs":5,
		"stop_sequences":["END","STOP"],
		"parallel_tool_calls":true,
		"user_id":"user_123",
		"reasoning":{"effort":"high"}
	}`), RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := decodeDeepSeekRequest(t, translated)
	if got["model"] != "deepseek-chat" || got["stream"] != true || got["temperature"] != 0.2 || got["top_p"] != 0.8 || got["max_tokens"] != float64(512) {
		t.Fatalf("translated controls = %#v", got)
	}
	if got["logprobs"] != true || got["top_logprobs"] != float64(5) || got["parallel_tool_calls"] != true || got["user"] != "user_123" {
		t.Fatalf("translated optional controls = %#v", got)
	}
	if got["reasoning_effort"] != "high" {
		t.Fatalf("reasoning effort = %#v", got)
	}
	thinking, ok := got["thinking"].(map[string]any)
	if !ok || thinking["type"] != "enabled" {
		t.Fatalf("thinking = %#v", got["thinking"])
	}
	stop := got["stop"].([]any)
	if len(stop) != 2 || stop[0] != "END" || stop[1] != "STOP" {
		t.Fatalf("stop = %#v", stop)
	}
	messages := got["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" || messages[0].(map[string]any)["content"] != "system rules" || messages[1].(map[string]any)["content"] != "hello" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestDeepSeekResponsesRequestReasoningEffortMapping(t *testing.T) {
	fixtures := []struct {
		effort, wantReasoning, wantThinking string
	}{
		{"minimal", "", "disabled"},
		{"low", "high", "enabled"},
		{"medium", "high", "enabled"},
		{"high", "high", "enabled"},
		{"xhigh", "max", "enabled"},
		{"max", "max", "enabled"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.effort, func(t *testing.T) {
			body := `{"input":"hello","reasoning":{"effort":"` + fixture.effort + `"}}`
			translated, err := ResponsesRequest([]byte(body), RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got := decodeDeepSeekRequest(t, translated)
			if fixture.wantReasoning == "" {
				if _, found := got["reasoning_effort"]; found {
					t.Fatalf("unexpected reasoning_effort: %#v", got)
				}
			} else if got["reasoning_effort"] != fixture.wantReasoning {
				t.Fatalf("reasoning = %#v", got)
			}
			thinking := got["thinking"].(map[string]any)
			if thinking["type"] != fixture.wantThinking {
				t.Fatalf("thinking = %#v", thinking)
			}
		})
	}
}

func TestDeepSeekResponsesRequestMapsJSONModeAndModelOverride(t *testing.T) {
	translated, err := ResponsesRequest([]byte(`{"model":"request-model","input":"hello","response_format":{"type":"json_schema"}}`), RequestOptions{Model: "override-model"})
	if err != nil {
		t.Fatal(err)
	}
	got := decodeDeepSeekRequest(t, translated)
	if got["model"] != "override-model" {
		t.Fatalf("model = %#v", got["model"])
	}
	format, ok := got["response_format"].(map[string]any)
	if !ok || format["type"] != "json_object" {
		t.Fatalf("response_format = %#v", got["response_format"])
	}
	messages := got["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" || messages[0].(map[string]any)["content"] != "Respond with valid JSON only." {
		t.Fatalf("JSON-mode messages = %#v", messages)
	}
}

func TestDeepSeekResponsesRequestCanonicalizesModelAliases(t *testing.T) {
	for _, test := range []struct {
		name, body, override, want string
	}{
		{name: "pro body alias", body: `{"model":"pro","input":"hello"}`, want: "deepseek-v4-pro"},
		{name: "case folded flash body alias", body: `{"model":"FLASH","input":"hello"}`, want: "deepseek-v4-flash"},
		{name: "auto override alias", body: `{"input":"hello"}`, override: "auto", want: "deepseek-v4-pro"},
		{name: "custom model preserved", body: `{"model":"custom-model","input":"hello"}`, want: "custom-model"},
		{name: "override keeps precedence", body: `{"model":"pro","input":"hello"}`, override: "custom-model", want: "custom-model"},
	} {
		t.Run(test.name, func(t *testing.T) {
			translated, err := ResponsesRequest([]byte(test.body), RequestOptions{Model: test.override})
			if err != nil {
				t.Fatal(err)
			}
			if got := decodeDeepSeekRequest(t, translated)["model"]; got != test.want {
				t.Fatalf("translated model = %#v, want %q", got, test.want)
			}
		})
	}
}

func TestDeepSeekResponsesRequestTranslatesHistoryAndRoles(t *testing.T) {
	translated, err := ResponsesRequest([]byte(`{
		"input":[
			{"type":"message","role":"developer","content":"dev rules"},
			{"type":"message","role":"critic","content":"critic note"},
			{"type":"function_call","call_id":"call_1","name":"exec","arguments":{"cmd":"ls"}},
			{"type":"function_call_output","call_id":"call_1","output":{"ok":true}},
			{"type":"local_shell_call","call_id":"call_2","action":{"command":["pwd"]}}
		]
	}`), RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	messages := decodeDeepSeekRequest(t, translated)["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].(map[string]any)["role"] != "system" || messages[1].(map[string]any)["role"] != "user" {
		t.Fatalf("role mapping = %#v", messages[:2])
	}
	call := messages[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if call["name"] != "exec" || call["arguments"] != `{"cmd":"rtk ls"}` {
		t.Fatalf("function call = %#v", call)
	}
	output := messages[3].(map[string]any)
	if output["role"] != "tool" || output["tool_call_id"] != "call_1" || output["content"] != `{"ok":true}` {
		t.Fatalf("tool output = %#v", output)
	}
}

func TestDeepSeekResponsesRequestRejectsUnsupportedControls(t *testing.T) {
	fixtures := []struct {
		body, want string
	}{
		{`{"input":"x","frequency_penalty":1}`, "frequency_penalty"},
		{`{"input":"x","n":2}`, "n is not supported"},
		{`{"input":"x","parallel_tool_calls":false}`, "parallel_tool_calls=false"},
		{`{"input":"x","top_logprobs":2}`, "requires logprobs=true"},
		{`{"input":"x","stop_sequences":["1","2","3","4","5","6","7","8","9","10","11","12","13","14","15","16","17"]}`, "at most 16"},
		{`{"input":"x","reasoning":{"summary":"auto"}}`, "reasoning.summary"},
		{`{"input":[{"type":"message","content":[{"type":"input_image","image_url":"x"}]}]}`, "text-only"},
	}
	for _, fixture := range fixtures {
		if _, err := ResponsesRequest([]byte(fixture.body), RequestOptions{}); err == nil || !strings.Contains(err.Error(), fixture.want) {
			t.Fatalf("body %s error = %v, want contains %q", fixture.body, err, fixture.want)
		}
	}
}

func TestDeepSeekResponsesRequestMapsWebSearch(t *testing.T) {
	empty, err := ResponsesRequest([]byte(`{"input":"x","web_search_options":{}}`), RequestOptions{})
	if err != nil || decodeDeepSeekRequest(t, empty)["web_search_options"] == nil {
		t.Fatalf("empty web-search options = %s, error = %v", empty, err)
	}

	translated, err := ResponsesRequest([]byte(`{"input":"x","web_search_options":{
		"search_context_size":"medium",
		"allowed_domains":["docs.example"],
		"blocked_domains":["ads.example"],
		"max_uses":2,
		"user_location":{"country":"US"},
		"provider_extension":true
	}}`), RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	options := decodeDeepSeekRequest(t, translated)["web_search_options"].(map[string]any)
	if options["search_context_size"] != "medium" || options["allowed_domains"].([]any)[0] != "docs.example" ||
		options["blocked_domains"].([]any)[0] != "ads.example" || options["max_uses"] != float64(2) ||
		options["user_location"].(map[string]any)["country"] != "US" || options["provider_extension"] != true {
		t.Fatalf("forwarded web-search options = %#v", options)
	}

	for _, kind := range []string{"web_search", "web_search_preview", "web_search_preview_2025_03_11"} {
		t.Run(kind, func(t *testing.T) {
			body := `{"input":"x","tools":[{"type":"` + kind + `","context_size":"high","location":{"country":"JP"}},` +
				`{"type":"function","name":"lookup"}]}`
			translated, err := ResponsesRequest([]byte(body), RequestOptions{})
			if err != nil {
				t.Fatal(err)
			}
			got := decodeDeepSeekRequest(t, translated)
			options := got["web_search_options"].(map[string]any)
			tools := got["tools"].([]any)
			if options["search_context_size"] != "high" || options["user_location"].(map[string]any)["country"] != "JP" ||
				len(tools) != 1 || tools[0].(map[string]any)["function"].(map[string]any)["name"] != "lookup" {
				t.Fatalf("translated web search = %#v", got)
			}
		})
	}

	translated, err = ResponsesRequest([]byte(`{"input":"x","web_search_options":{"search_context_size":"low"},`+
		`"tools":[{"type":"web_search","search_context_size":"high"}]}`), RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	options = decodeDeepSeekRequest(t, translated)["web_search_options"].(map[string]any)
	if options["search_context_size"] != "low" {
		t.Fatalf("explicit web-search options did not take precedence: %#v", options)
	}
}

func TestDeepSeekResponsesRequestRejectsWebSearchWhenModeOff(t *testing.T) {
	for _, body := range []string{
		`{"input":"search","web_search_options":{}}`,
		`{"input":"search","tools":[{"type":"web_search_preview"}]}`,
	} {
		_, err := ResponsesRequest([]byte(body), RequestOptions{WebSearchMode: "off"})
		if err == nil || !strings.Contains(err.Error(), "web search mode is off") {
			t.Fatalf("body %s error = %v", body, err)
		}
	}
}

func TestDeepSeekResponsesRequestRejectsMalformedWebSearch(t *testing.T) {
	fixtures := []struct {
		body, want string
	}{
		{`{"input":"x","web_search_options":null}`, "web_search_options must be an object"},
		{`{"input":"x","web_search_options":[]}`, "web_search_options must be an object"},
		{`{"input":"x","web_search_options":{"search_context_size":"huge"}}`, "search_context_size must be low, medium, or high"},
		{`{"input":"x","web_search_options":{"allowed_domains":"docs.example"}}`, "allowed_domains must be an array of strings"},
		{`{"input":"x","web_search_options":{"allowed_domains":[1]}}`, "allowed_domains entries must be non-empty strings"},
		{`{"input":"x","web_search_options":{"blocked_domains":["  "]}}`, "blocked_domains entries must be non-empty strings"},
		{`{"input":"x","web_search_options":{"max_uses":0}}`, "max_uses must be a positive integer"},
		{`{"input":"x","web_search_options":{"max_uses":1.5}}`, "max_uses must be a positive integer"},
		{`{"input":"x","web_search_options":{"user_location":[]}}`, "user_location must be an object"},
		{`{"input":"x","tools":[{"type":"web_search_preview","search_context_size":false}]}`, "web_search context_size must be low, medium, or high"},
		{`{"input":"x","tools":[{"type":"web_search","allowed_domains":[""]}]}`, "allowed_domains entries must be non-empty strings"},
	}
	for _, fixture := range fixtures {
		if _, err := ResponsesRequest([]byte(fixture.body), RequestOptions{}); err == nil || !strings.Contains(err.Error(), fixture.want) {
			t.Fatalf("body %s error = %v, want contains %q", fixture.body, err, fixture.want)
		}
	}
}

func decodeDeepSeekRequest(t *testing.T, content []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestDeepSeekTranslatedRequestPreservesResponseMetadata(t *testing.T) {
	translated, err := TranslateResponsesRequest([]byte(`{
		"input":"hello",
		"response_format":{"type":"json_schema"},
		"metadata":{"tag":"one","deepseek":{}},
		"client_metadata":{"client":true},
		"prompt_cache_key":" cache-key ",
		"prompt_cache_retention":"24h"
	}`), RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	metadata := translated.ResponseMetadata
	if metadata["tag"] != "one" || metadata["prompt_cache_key"] != " cache-key " || metadata["prompt_cache_retention"] != "24h" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata["client_metadata"].(map[string]any)["client"] != true {
		t.Fatalf("client metadata = %#v", metadata)
	}
	deepseek := metadata["deepseek"].(map[string]any)
	degraded := deepseek["degraded_response_format"].(map[string]any)
	if degraded["from"] != "json_schema" || !strings.Contains(degraded["reason"].(string), "JSON Schema") {
		t.Fatalf("degraded metadata = %#v", degraded)
	}
}

func TestDeepSeekTranslatedRequestMetadataErrorPrecedence(t *testing.T) {
	fixtures := []struct {
		body, want string
	}{
		{`{"input":"x","metadata":{"deepseek":"bad"},"client_metadata":[],"prompt_cache_key":42}`, "metadata.deepseek must be an object"},
		{`{"input":"x","metadata":{},"client_metadata":[],"prompt_cache_key":42}`, "client_metadata must be an object"},
		{`{"input":"x","metadata":{},"client_metadata":{},"prompt_cache_key":42}`, "prompt_cache_key must be a string"},
		{`{"input":"x","metadata":{},"client_metadata":{},"prompt_cache_key":"ok","prompt_cache_retention":42}`, "prompt_cache_retention must be a string"},
	}
	for _, fixture := range fixtures {
		if _, err := TranslateResponsesRequest([]byte(fixture.body), RequestOptions{}); err == nil || !strings.Contains(err.Error(), fixture.want) {
			t.Fatalf("body %s error = %v, want %q", fixture.body, err, fixture.want)
		}
	}
}
