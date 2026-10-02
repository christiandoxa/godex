package kiro

import (
	"strings"
	"testing"
)

func TestParseKiroResponsesRequestExtractsReasoningEffort(t *testing.T) {
	request, err := parseKiroResponsesRequest([]byte(`{"model":"luna","reasoning":{"effort":"medium"},"input":"hello"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	if request.effort != "medium" || request.model != "luna" {
		t.Fatalf("request = %#v", request)
	}
}

func TestKiroParentAndSubAgentStripExternalToolControls(t *testing.T) {
	parentBody := []byte("{\"model\":\"auto\",\"input\":\"inspect repository\",\"parallel_tool_calls\":false,\"tools\":[{\"type\":\"function\",\"name\":\"shell\"}],\"functions\":[{\"name\":\"shell\"}]}")
	if _, err := parseKiroResponsesRequestForAgent(parentBody, false, false); err == nil || !strings.Contains(err.Error(), "parallel_tool_calls=false") {
		t.Fatalf("parent error = %v", err)
	}
	subAgentBody := []byte("{\"model\":\"auto\",\"input\":\"inspect repository\",\"parallel_tool_calls\":false,\"tool_choice\":\"required\",\"tools\":[{\"type\":\"function\",\"name\":\"shell\"}],\"functions\":[{\"name\":\"shell\"}],\"function_call\":\"auto\",\"web_search_options\":{}}")
	request, err := parseKiroResponsesRequestForAgent(subAgentBody, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if request.model != "auto" || !strings.Contains(request.prompt, "inspect repository") {
		t.Fatalf("sub-agent request = %#v", request)
	}
}

func TestKiroChatSubAgentStripsLegacyToolControls(t *testing.T) {
	body := []byte("{\"model\":\"auto\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}],\"parallel_tool_calls\":false,\"tool_choice\":\"required\",\"tools\":[{\"type\":\"function\",\"function\":{\"name\":\"shell\"}}],\"functions\":[{\"name\":\"shell\"}],\"function_call\":\"auto\",\"web_search_options\":{}}")
	request, err := parseKiroChatRequestForAgent(body, true)
	if err != nil {
		t.Fatal(err)
	}
	if request.prompt != "User:\nhello" {
		t.Fatalf("prompt = %q", request.prompt)
	}
}

func TestKiroRequestValidationCodesMatchProdex04351(t *testing.T) {
	chat := []struct {
		name, body, code, message string
	}{
		{"response format precedence", `{"messages":[],"response_format":{"type":"json_object"},"n":2,"stop":["end"]}`, "unsupported_response_format", "only supports chat response_format type 'text'"},
		{"choice count", `{"messages":[{"role":"user","content":"hello"}],"n":2}`, "unsupported_choice_count", "parameter n=1"},
		{"stop", `{"messages":[{"role":"user","content":"hello"}],"stop":["END"]}`, "unsupported_stop", "stop sequences"},
		{"temperature", `{"messages":[{"role":"user","content":"hello"}],"temperature":0.5}`, "unsupported_temperature", "temperature"},
		{"top p", `{"messages":[{"role":"user","content":"hello"}],"top_p":0.5}`, "unsupported_top_p", "top_p"},
		{"presence", `{"messages":[{"role":"user","content":"hello"}],"presence_penalty":1}`, "unsupported_presence_penalty", "presence_penalty"},
		{"frequency", `{"messages":[{"role":"user","content":"hello"}],"frequency_penalty":1}`, "unsupported_frequency_penalty", "frequency_penalty"},
		{"seed", `{"messages":[{"role":"user","content":"hello"}],"seed":7}`, "unsupported_seed", "seed"},
		{"parallel", `{"messages":[{"role":"user","content":"hello"}],"parallel_tool_calls":false}`, "unsupported_parallel_tool_calls", "parallel_tool_calls"},
		{"token invalid", `{"messages":[{"role":"user","content":"hello"}],"max_completion_tokens":0}`, "unsupported_token_limit", "must be a positive integer"},
		{"token valid but unsupported", `{"messages":[{"role":"user","content":"hello"}],"max_tokens":32}`, "unsupported_token_limit", "max_tokens control"},
		{"invalid logprobs", `{"messages":[{"role":"user","content":"hello"}],"logprobs":"yes"}`, "invalid_logprobs", "must be a boolean"},
	}
	for _, fixture := range chat {
		t.Run("chat "+fixture.name, func(t *testing.T) {
			_, err := parseKiroChatRequestForAgent([]byte(fixture.body), false)
			assertKiroRequestError(t, err, fixture.code, fixture.message)
		})
	}

	responses := []struct {
		name, body, code, message string
	}{
		{"generation precedence", `{"model":"auto","input":"hello","temperature":0.5,"stop":["end"],"logprobs":"yes"}`, "unsupported_generation_control", "temperature control"},
		{"token", `{"model":"auto","input":"hello","max_output_tokens":64}`, "unsupported_token_limit", "max_output_tokens control"},
		{"stop", `{"model":"auto","input":"hello","stop_sequences":["end"]}`, "unsupported_stop", "stop-sequence controls"},
		{"invalid logprobs", `{"model":"auto","input":"hello","logprobs":"yes"}`, "invalid_logprobs", "must be a boolean"},
		{"logprobs", `{"model":"auto","input":"hello","logprobs":true}`, "unsupported_logprobs", "log probabilities"},
		{"top logprobs", `{"model":"auto","input":"hello","top_logprobs":1}`, "unsupported_logprobs", "top_logprobs"},
		{"format", `{"model":"auto","input":"hello","text":{"format":{"type":"json_schema"}}}`, "unsupported_response_format", "text response format"},
		{"tool choice", `{"model":"auto","input":"hello","tool_choice":"required"}`, "unsupported_tool_choice", "owns tool selection"},
		{"web search", `{"model":"auto","input":"hello","web_search_options":{}}`, "unsupported_web_search_options", "owns web search"},
		{"parallel", `{"model":"auto","input":"hello","parallel_tool_calls":false}`, "unsupported_parallel_tool_calls", "parallel_tool_calls=false"},
		{"effort", `{"model":"auto","input":"hello","reasoning":{"effort":"ultra"}}`, "unsupported_reasoning_effort", "reasoning effort `ultra`"},
		{"reasoning shape before effort", `{"model":"auto","input":"hello","reasoning":{"other":true,"effort":"bogus"}}`, "invalid_request", "reasoning.other"},
	}
	for _, fixture := range responses {
		t.Run("responses "+fixture.name, func(t *testing.T) {
			_, err := parseKiroResponsesRequestForAgent([]byte(fixture.body), false, false)
			assertKiroRequestError(t, err, fixture.code, fixture.message)
		})
	}
}

func TestKiroRequestValidationAcceptsProdexNoopControls(t *testing.T) {
	request, err := parseKiroChatRequestForAgent([]byte(`{"model":"auto","messages":[{"role":"user","content":"hello"}],"stop":[],"temperature":1,"top_p":1,"presence_penalty":0,"frequency_penalty":0,"parallel_tool_calls":true,"user":"user-123"}`), false)
	if err != nil || request.prompt != "User:\nhello" {
		t.Fatalf("noop chat controls = %#v, err=%v", request, err)
	}
	if _, err := parseKiroResponsesRequestForAgent([]byte(`{"model":"gpt-5.6-luna","input":"hello","reasoning":{"effort":"none"}}`), false, false); err != nil {
		t.Fatalf("catalogued none reasoning effort rejected: %v", err)
	}
	if _, err := parseKiroMessagesRequestForAgent([]byte(`{"model":"auto","max_tokens":0,"messages":[{"role":"user","content":"hello"}]}`), false); err != nil {
		t.Fatalf("Messages token limit compatibility rejected: %v", err)
	}
}

func TestKiroRequestBodyShapeCodes(t *testing.T) {
	_, err := parseKiroChatRequestForAgent([]byte(`[]`), false)
	assertKiroRequestError(t, err, "invalid_request_body", "must be a JSON object")
	_, err = parseKiroResponsesRequestForAgent([]byte(`{bad`), false, false)
	assertKiroRequestError(t, err, "invalid_json", "must be valid JSON")
	_, err = parseKiroChatRequestForAgent([]byte(`{"model":"auto"}`), false)
	assertKiroRequestError(t, err, "missing_messages", "missing messages")
}

func assertKiroRequestError(t *testing.T, err error, code, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s error", code)
	}
	if got := kiroRequestErrorCode(err); got != code {
		t.Fatalf("error code = %q, want %q; err=%v", got, code, err)
	}
	if !strings.Contains(err.Error(), message) {
		t.Fatalf("error message = %q, want contains %q", err.Error(), message)
	}
}

func TestKiroChatPromptPreservesToolHistory(t *testing.T) {
	request, err := parseKiroChatRequestForAgent([]byte(`{
		"model":"auto",
		"messages":[
			{"role":"assistant","content":"checking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.go\"}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"file contents"},
			{"role":"assistant","content":null,"function_call":{"name":"legacy_lookup","arguments":"{\"q\":\"x\"}"}}
		]
	}`), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Assistant:\nchecking\nTool call read_file: {\"path\":\"a.go\"}",
		"Tool:\nfile contents",
		"Assistant:\nTool call legacy_lookup: {\"q\":\"x\"}",
	} {
		if !strings.Contains(request.prompt, expected) {
			t.Fatalf("prompt missing %q: %q", expected, request.prompt)
		}
	}
}

func TestKiroChatLegacyFunctionSelectionMatchesResponsesPolicy(t *testing.T) {
	if _, err := parseKiroChatRequestForAgent([]byte(`{"messages":[{"role":"user","content":"hello"}],"function_call":{"name":"read_file"}}`), false); err == nil {
		t.Fatal("named legacy function_call unexpectedly accepted")
	} else {
		assertKiroRequestError(t, err, "unsupported_tool_choice", "owns tool selection")
	}
	if _, err := parseKiroChatRequestForAgent([]byte(`{"messages":[{"role":"user","content":"hello"}],"function_call":"auto"}`), false); err != nil {
		t.Fatalf("legacy function_call=auto rejected: %v", err)
	}
	if _, err := parseKiroChatRequestForAgent([]byte(`{"messages":[{"role":"user","content":"hello"}],"function_call":"none"}`), false); err == nil {
		t.Fatal("legacy function_call=none unexpectedly accepted")
	} else {
		assertKiroRequestError(t, err, "unsupported_tool_choice", "owns tool selection")
	}
	if _, err := parseKiroChatRequestForAgent([]byte(`{"messages":[{"role":"user","content":"hello"}],"function_call":"legacy-arbitrary"}`), false); err != nil {
		t.Fatalf("arbitrary legacy function_call string should be ignored: %v", err)
	}
}
