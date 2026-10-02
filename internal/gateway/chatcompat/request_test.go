package chatcompat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesRequestMatchesProdexFixtures(t *testing.T) {
	body := []byte(`{"model":"request-model","instructions":"system","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"héllo 東京"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"max_output_tokens":17,"stream":true}`)
	translated, err := ResponsesRequest(body, "default-model", "")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "request-model" || got["max_tokens"] != float64(17) || got["stream"] != true {
		t.Fatalf("translated = %#v", got)
	}
	messages := got["messages"].([]any)
	if len(messages) != 3 || messages[0].(map[string]any)["role"] != "system" || messages[1].(map[string]any)["content"] != "héllo 東京" || messages[2].(map[string]any)["content"] != "done" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestResponsesRequestTranslatesToolHistory(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","call_id":"c1","namespace":"agents","name":"run","arguments":{"x":1,"z":2}},{"type":"function_call_output","call_id":"c1","output":{"ok":true}}]}`)
	translated, err := ResponsesRequest(body, "default-model", "")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	messages := got["messages"].([]any)
	call := messages[0].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if call["name"] != "agents.run" || call["arguments"] != `{"x":1,"z":2}` {
		t.Fatalf("call = %#v", call)
	}
	tool := messages[1].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "c1" || tool["content"] != `{"ok":true}` {
		t.Fatalf("tool = %#v", tool)
	}
}

func TestResponsesRequestTranslatesProdexToolHistory(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"input_text","text":"hello"},
		{"type":"custom_tool_call","call_id":"call_patch","name":"apply_patch","input":"*** Begin Patch"},
		{"type":"mcp_call","id":"call_mcp","name":"mcp__files__read","arguments":{"path":"README.md"},"output":"file contents"},
		{"type":"local_shell_call","call_id":"call_shell","action":{"command":["git","status"],"cwd":"/work","timeout":10,"env":{"MODE":"test"}}},
		{"type":"custom_tool_call_output","call_id":"call_patch","output":"patched"},
		{"type":"mcp_tool_result","call_id":"call_result","content":[{"type":"output_text","text":"first"},{"type":"input_text","text":"second"}]},
		{"type":"mcp_call_output","tool_call_id":"call_error","error":"failed"}
	]}`)
	translated, err := ResponsesRequest(body, "default-model", "")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	messages := got["messages"].([]any)
	if len(messages) != 8 {
		t.Fatalf("messages = %#v", messages)
	}
	message := func(index int) map[string]any { return messages[index].(map[string]any) }
	call := func(index int) map[string]any {
		calls := message(index)["tool_calls"].([]any)
		return calls[0].(map[string]any)
	}
	function := func(index int) map[string]any { return call(index)["function"].(map[string]any) }
	if message(0)["role"] != "user" || message(0)["content"] != "hello" {
		t.Fatalf("loose text message = %#v", message(0))
	}
	if call(1)["id"] != "call_patch" || function(1)["name"] != "apply_patch" || function(1)["arguments"] != `{"input":"*** Begin Patch"}` {
		t.Fatalf("custom tool call = %#v", message(1))
	}
	if call(2)["id"] != "call_mcp" || function(2)["name"] != "mcp__files__read" || function(2)["arguments"] != `{"path":"README.md"}` {
		t.Fatalf("MCP call = %#v", message(2))
	}
	if message(3)["role"] != "tool" || message(3)["tool_call_id"] != "call_mcp" || message(3)["content"] != "file contents" {
		t.Fatalf("MCP call output = %#v", message(3))
	}
	if call(4)["id"] != "call_shell" || function(4)["name"] != "shell_command" {
		t.Fatalf("local shell call = %#v", message(4))
	}
	var shellArguments map[string]any
	if err := json.Unmarshal([]byte(function(4)["arguments"].(string)), &shellArguments); err != nil {
		t.Fatal(err)
	}
	if shellArguments["command"] != "git status" || shellArguments["cwd"] != "/work" || shellArguments["timeout"] != float64(10) ||
		shellArguments["env"].(map[string]any)["MODE"] != "test" {
		t.Fatalf("local shell arguments = %#v", shellArguments)
	}
	if message(5)["tool_call_id"] != "call_patch" || message(5)["content"] != "patched" {
		t.Fatalf("custom tool output = %#v", message(5))
	}
	if message(6)["tool_call_id"] != "call_result" || message(6)["content"] != "first\nsecond" {
		t.Fatalf("MCP tool result = %#v", message(6))
	}
	if message(7)["tool_call_id"] != "call_error" || message(7)["content"] != "failed" {
		t.Fatalf("MCP call output = %#v", message(7))
	}
}

func TestResponsesRequestRejectsMalformedProdexToolHistory(t *testing.T) {
	for _, fixture := range []struct {
		body string
		want string
	}{
		{`{"input":[{"type":"custom_tool_call","name":"apply_patch","input":"patch"}]}`, "require a call_id"},
		{`{"input":[{"type":"mcp_call","call_id":"c1"}]}`, "require a function name"},
		{`{"input":[{"type":"local_shell_call","call_id":"c1"}]}`, "requires a command"},
		{`{"input":[{"type":"mcp_tool_result","call_id":"c1"}]}`, "require output content"},
	} {
		if _, err := ResponsesRequest([]byte(fixture.body), "default-model", ""); err == nil || !strings.Contains(err.Error(), fixture.want) {
			t.Fatalf("body %s error = %v, want contains %q", fixture.body, err, fixture.want)
		}
	}
}

func TestResponsesRequestRejectsUnsupportedControlsInProdexOrder(t *testing.T) {
	fixtures := []struct {
		body, want string
	}{
		{`{"messages":[],"response_format":{},"input":"x"}`, "expects Responses input"},
		{`{"response_format":{},"reasoning":{},"input":"x"}`, "response_format"},
		{`{"reasoning":{},"previous_response_id":"r","input":"x"}`, "reasoning"},
		{`{"previous_response_id":"r","input":"x"}`, "previous_response_id"},
		{`{"text":{"format":{}},"n":2,"input":"x"}`, "text.format"},
		{`{"n":2,"input":"x"}`, "n>1"},
		{`{"tools":[{"type":"custom"}],"input":"x"}`, "function tools"},
		{`{"parallel_tool_calls":false,"input":"x"}`, "parallel_tool_calls=false"},
		{`{"input":[{"type":"input_image","image_url":"x"}]}`, "message/function-call"},
		{`{"input":42}`, "textual input"},
	}
	for _, fixture := range fixtures {
		if _, err := ResponsesRequest([]byte(fixture.body), "default-model", ""); err == nil || !strings.Contains(err.Error(), fixture.want) {
			t.Fatalf("body %s error = %v, want contains %q", fixture.body, err, fixture.want)
		}
	}
}

func TestResponsesRequestCallerModelOverridesInvalidRequestModel(t *testing.T) {
	translated, err := ResponsesRequest([]byte(`{"input":"hello","model":42,"stream":"true","max_output_tokens":false}`), "default-model", "caller-model")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "caller-model" || got["stream"] != false || got["max_tokens"] != false {
		t.Fatalf("translated = %#v", got)
	}
}
