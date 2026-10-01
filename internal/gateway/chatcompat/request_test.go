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
