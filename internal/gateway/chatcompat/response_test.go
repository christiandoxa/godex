package chatcompat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChatResponseMapsTextToolsUsageAndRTK(t *testing.T) {
	body := []byte(`{"id":"chatcmpl_test","model":"compat-model","created":1700000000,"choices":[{"message":{"role":"assistant","content":[{"text":"hello"},{"content":"world"},{"text":""}],"tool_calls":[{"id":"call_test","function":{"name":"functions.exec_command","arguments":"{\"cmd\":\"ls\"}"}}]}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
	translated, err := ChatResponse(body, time.Unix(1700000123, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "chatcmpl_test" || got["model"] != "compat-model" || got["created_at"] != float64(1700000000) {
		t.Fatalf("response = %#v", got)
	}
	output := got["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("output = %#v", output)
	}
	tool := output[1].(map[string]any)
	if tool["namespace"] != "functions" || tool["name"] != "exec_command" || tool["arguments"] != `{"cmd":"rtk ls"}` {
		t.Fatalf("tool = %#v", tool)
	}
	usage := got["usage"].(map[string]any)
	if usage["input_tokens"] != float64(3) || usage["output_tokens"] != float64(4) || usage["total_tokens"] != float64(7) {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestChatResponseSparseDefaults(t *testing.T) {
	translated, err := ChatResponse([]byte(`{"choices":[]}`), time.Unix(1700000123, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "resp_prodex" || got["model"] != "unknown" || got["created_at"] != float64(1700000123) || len(got["output"].([]any)) != 0 {
		t.Fatalf("response = %#v", got)
	}
}

func TestRTKCommandWrappingMatchesNoisySegments(t *testing.T) {
	for input, want := range map[string]string{
		"ls":                         "rtk ls",
		"pwd":                        "pwd",
		"cd /repo && cargo check -q": "cd /repo && rtk cargo check -q",
		"rtk ls":                     "rtk ls",
	} {
		got, _ := wrapRTKCommand(input)
		if got != want {
			t.Fatalf("wrap %q = %q, want %q", input, got, want)
		}
	}
}

func TestChatResponseWithOptionsMapsProviderMetadataAndUsageDetails(t *testing.T) {
	body := []byte(`{"id":"chatcmpl_1","model":"deepseek-v4-pro","created":1700000000,"choices":[{"message":{"content":"done","reasoning_content":"thought","refusal":"no","annotations":[{"type":"note"}],"tool_calls":[{"id":"call_1","function":{"name":"mcp__prodex_sqz__sqz_read_file","arguments":"{\"path\":\"README.md\"}"},"extra_content":{"google":{"thought_signature":"sig_1"}}}]},"finish_reason":"tool_calls","logprobs":{"tokens":[]}}],"system_fingerprint":"fp_1","usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18,"prompt_cache_hit_tokens":5,"prompt_cache_miss_tokens":6,"completion_tokens_details":{"reasoning_tokens":2}}}`)
	translated, err := ChatResponseWithOptions(body, time.Unix(1700000123, 0), ResponseOptions{
		ProviderKey: "deepseek", AdapterLabel: "DeepSeek", DefaultModel: "deepseek-v4-pro",
		FallbackResponseID: func() string { return "resp_deepseek_fallback" },
		FallbackCallID:     func(index int) string { return "call_deepseek_fallback" },
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	metadata := got["metadata"].(map[string]any)["deepseek"].(map[string]any)
	if metadata["reasoning_content"] != "thought" || metadata["refusal"] != "no" || metadata["finish_reason"] != "tool_calls" || metadata["system_fingerprint"] != "fp_1" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if _, ok := metadata["logprobs"].(map[string]any); !ok || len(metadata["annotations"].([]any)) != 1 {
		t.Fatalf("metadata extras = %#v", metadata)
	}
	usage := got["usage"].(map[string]any)
	if usage["input_tokens"] != float64(11) || usage["output_tokens"] != float64(7) || usage["total_tokens"] != float64(18) {
		t.Fatalf("usage = %#v", usage)
	}
	if usage["input_tokens_details"].(map[string]any)["cached_tokens"] != float64(5) || usage["output_tokens_details"].(map[string]any)["reasoning_tokens"] != float64(2) {
		t.Fatalf("usage details = %#v", usage)
	}
	cache := usage["metadata"].(map[string]any)["deepseek"].(map[string]any)
	if cache["prompt_cache_hit_tokens"] != float64(5) || cache["prompt_cache_miss_tokens"] != float64(6) {
		t.Fatalf("usage cache metadata = %#v", cache)
	}
	output := got["output"].([]any)
	tool := output[1].(map[string]any)
	if tool["namespace"] != "mcp__prodex_sqz" || tool["name"] != "sqz_read_file" || tool["gemini_thought_signature"] != "sig_1" {
		t.Fatalf("tool = %#v", tool)
	}
}

func TestChatResponseWithOptionsFailsInvalidToolCall(t *testing.T) {
	for _, fixture := range []struct {
		name, body, want string
	}{
		{"missing function", `{"choices":[{"message":{"tool_calls":[{"id":"call_1"}]}}]}`, "without a function object"},
		{"missing name", `{"choices":[{"message":{"tool_calls":[{"id":"call_1","function":{"arguments":"{}"}}]}}]}`, "without a function name"},
		{"invalid arguments", `{"choices":[{"message":{"tool_calls":[{"id":"call_1","function":{"name":"lookup","arguments":"{"}}]}}]}`, "malformed JSON arguments"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			translated, err := ChatResponseWithOptions([]byte(fixture.body), time.Unix(1, 0), ResponseOptions{ProviderKey: "deepseek", AdapterLabel: "DeepSeek"})
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(translated, &got); err != nil {
				t.Fatal(err)
			}
			if got["status"] != "failed" || got["error"].(map[string]any)["code"] != "invalid_tool_call_arguments" || !strings.Contains(got["error"].(map[string]any)["message"].(string), fixture.want) {
				t.Fatalf("response = %#v", got)
			}
			if len(got["output"].([]any)) != 0 {
				t.Fatalf("invalid tool response output = %#v", got["output"])
			}
		})
	}
}

func TestChatResponseWithOptionsUsesFallbackIDs(t *testing.T) {
	translated, err := ChatResponseWithOptions([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"shell","arguments":"{}"}}]}}]}`), time.Unix(7, 0), ResponseOptions{
		ProviderKey: "deepseek", AdapterLabel: "DeepSeek", DefaultModel: "deepseek-chat",
		FallbackResponseID: func() string { return "resp_deepseek_request-v7" },
		FallbackCallID:     func(index int) string { return "call_deepseek_call-v7" },
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "resp_deepseek_request-v7" || got["model"] != "deepseek-chat" || got["output"].([]any)[0].(map[string]any)["call_id"] != "call_deepseek_call-v7" {
		t.Fatalf("fallback IDs = %#v", got)
	}
}
