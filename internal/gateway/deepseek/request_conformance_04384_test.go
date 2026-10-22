package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekSimpleRequestPolicy(t *testing.T) {
	for _, fixture := range []struct {
		body   string
		simple bool
	}{
		{`{"input":"hello"}`, true}, {`{"input":[]}`, true},
		{`{"input":"hello","previous_response_id":"bound"}`, false},
		{`{"input":"hello","previous_response_id":"unknown"}`, true},
		{`{"input":"hello","web_search_options":null}`, false},
		{`{"input":"hello","safety_identifier":"synthetic_user"}`, false},
		{`{"input":"hello","tools":[]}`, true},
		{`{"input":"hello","tools":[{"type":"function","function":{"name":"lookup"}}]}`, true},
		{`{"input":"hello","tools":[{"type":"function","name":"lookup"}]}`, false},
		{`{"input":"hello","tool_choice":{"type":"function","name":"lookup"}}`, true},
		{`{"input":"hello","tool_choice":{"type":"function","function":{"name":"lookup"}}}`, false},
		{`{"input":"hello","response_format":{"type":"json_schema"}}`, true},
		{`{"input":"hello","response_format":null}`, false},
		{`{"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`, true},
		{`{"input":[{"role":"developer","content":"hello"}]}`, false},
		{`{"input":[{"role":"user","content":null}]}`, false},
		{`{"input":[{"content":[{"text":null,"input_text":"ignored"}]}]}`, false},
		{`{"input":[{"type":"function_call_output","call_id":"call-synthetic","output":null}]}`, true},
		{`{"input":[{"type":"function_call_output","id":"call-synthetic","output":"hello"}]}`, false},
		{`{"input":[{"type":"mcp_call_output","id":"call-synthetic","result":false}]}`, true},
		{`{"input":[{"type":"mcp_call_output","call_id":null,"id":"call-synthetic","result":false}]}`, false},
		{`{"input":[{"type":"function_call","name":"lookup","call_id":"call-synthetic"}]}`, true},
		{`{"input":[{"type":"function_call","name":null,"tool_name":"lookup"}]}`, false},
		{`{"input":[{"type":"local_shell_call","action":{"command":["echo","synthetic"]}}]}`, true},
		{`{"input":[{"type":"local_shell_call","action":{"command":["echo",2]}}]}`, false},
		{`{"input":[{"type":"image","content":"hello"}]}`, false},
	} {
		t.Run(fixture.body, func(t *testing.T) {
			var request map[string]any
			if err := json.Unmarshal([]byte(fixture.body), &request); err != nil {
				t.Fatal(err)
			}
			if got := deepSeekSimpleRequest04384(request, func(id string) bool { return id == "bound" }); got != fixture.simple {
				t.Fatalf("simple=%v, want %v", got, fixture.simple)
			}
		})
	}
}

func TestProdex04384DeepSeekEmittedBodyKeepsIndependentPendingHistory(t *testing.T) {
	history := []any{map[string]any{"role": "user", "content": "history request"}, map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call-history", "type": "function", "function": map[string]any{"name": "lookup", "arguments": "{}"}}}}}
	body := []byte(`{"model":"deepseek-v4-pro","input":[{"type":"function_call_output","call_id":"call-history","output":"synthetic result"}]}`)
	translated, err := translateResponsesRequestWithHistory(body, RequestOptions{}, history, func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"content":"synthetic result","role":"tool","tool_call_id":"call-history"}],"model":"deepseek-v4-pro","stream":false}`
	if string(translated.Body) != want || len(translated.conversationMessages) != 3 {
		t.Fatalf("emitted=%s pending=%#v", translated.Body, translated.conversationMessages)
	}
	translated.conversationMessages[0].(map[string]any)["content"] = "changed"
	if history[0].(map[string]any)["content"] != "history request" {
		t.Fatal("pending history mutated the stored input")
	}
}
