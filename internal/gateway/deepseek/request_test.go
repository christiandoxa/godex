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
	if len(messages) != 5 {
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
	shell := messages[4].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if shell["name"] != "exec_command" || shell["arguments"] != `{"cmd":"pwd"}` {
		t.Fatalf("shell call = %#v", shell)
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
		{`{"input":"x","web_search_options":{}}`, "web_search_options"},
		{`{"input":"x","previous_response_id":"resp_1"}`, "previous_response_id"},
		{`{"input":[{"type":"message","content":[{"type":"input_image","image_url":"x"}]}]}`, "text-only"},
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
