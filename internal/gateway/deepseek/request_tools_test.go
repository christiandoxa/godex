package deepseek

import (
	"strings"
	"testing"
)

func TestDeepSeekStrictToolsRequireRuntimeOptIn(t *testing.T) {
	body := []byte(`{"input":"hello","tools":[{"type":"function","name":"lookup","strict":true,"parameters":{"type":"object","properties":{"query":{"type":"string"}}}}]}`)
	if _, err := ResponsesRequest(body, RequestOptions{}); err == nil || !strings.Contains(err.Error(), "requires deepseek.strict_tools=true") {
		t.Fatalf("strict-tools disabled error = %v", err)
	}
	translated, err := ResponsesRequest(body, RequestOptions{StrictTools: true})
	if err != nil {
		t.Fatal(err)
	}
	got := decodeDeepSeekRequest(t, translated)
	tool := got["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if tool["strict"] != true {
		t.Fatalf("strict tool = %#v", tool)
	}
	schema := tool["parameters"].(map[string]any)
	if schema["additionalProperties"] != false {
		t.Fatalf("strict schema = %#v", schema)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("strict required = %#v", required)
	}
}

func TestDeepSeekStrictToolsRejectUnsupportedSchemaKeywords(t *testing.T) {
	body := []byte(`{"input":"hello","tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"query":{"type":"string","minLength":1}}}}]}`)
	if _, err := ResponsesRequest(body, RequestOptions{StrictTools: true}); err == nil || !strings.Contains(err.Error(), "unsupported keyword `minLength`") {
		t.Fatalf("strict schema error = %v", err)
	}
}

func TestDeepSeekToolChoiceAndThinkingPolicy(t *testing.T) {
	body := []byte(`{"input":"hello","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"tool_choice":{"type":"function","name":"lookup"}}`)
	translated, err := ResponsesRequest(body, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := decodeDeepSeekRequest(t, translated)
	choice := got["tool_choice"].(map[string]any)
	if choice["type"] != "function" || choice["function"].(map[string]any)["name"] != "lookup" {
		t.Fatalf("tool choice = %#v", choice)
	}

	thinkingBody := []byte(`{"input":"hello","reasoning":{"effort":"high"},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"tool_choice":{"type":"function","name":"lookup"}}`)
	translated, err = ResponsesRequest(thinkingBody, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got = decodeDeepSeekRequest(t, translated)
	if _, found := got["tool_choice"]; found {
		t.Fatalf("thinking request unexpectedly forwarded tool_choice: %#v", got)
	}
}

func TestDeepSeekToolValidationMatchesReferencePolicy(t *testing.T) {
	fixtures := []struct {
		body, want string
	}{
		{`{"input":"x","tools":[{"type":"file_search"}]}`, "tool type `file_search` is not supported"},
		{`{"input":"x","tools":[{"type":"web_search_previewX"}]}`, "tool type `web_search_previewX` is not supported"},
		{`{"input":"x","tools":[{"type":"function","name":"bad.name"}]}`, "function tool names"},
		{`{"input":"x","tools":[{"type":"function","name":"lookup"}],"tool_choice":{"type":"function","name":"missing"}}`, "does not match any translated function tool"},
		{`{"input":"x","tools":[{"type":"function","name":"lookup"}],"tool_choice":"weird"}`, "tool_choice string `weird`"},
	}
	for _, fixture := range fixtures {
		if _, err := ResponsesRequest([]byte(fixture.body), RequestOptions{}); err == nil || !strings.Contains(err.Error(), fixture.want) {
			t.Fatalf("body %s error = %v, want contains %q", fixture.body, err, fixture.want)
		}
	}
}
