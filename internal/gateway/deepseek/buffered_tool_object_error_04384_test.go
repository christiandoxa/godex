package deepseek

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestProdex04384BufferedToolObjectSyntaxDiagnostics(t *testing.T) {
	for _, fixture := range []struct{ arguments, diagnostic string }{
		{`{bad`, "key must be a string at line 1 column 2"},
		{`{1:0}`, "key must be a string at line 1 column 2"},
		{"\n {bad", "key must be a string at line 2 column 3"},
		{`{"a":1,}`, "trailing comma at line 1 column 8"},
		{`{"a" 1}`, "expected `:` at line 1 column 6"},
		{`{"a":1 "b":2}`, "expected `,` or `}` at line 1 column 8"},
		{`{"雪":1, bad:2}`, "key must be a string at line 1 column 11"},
		{"{\"雪\":1,\n bad:2}", "key must be a string at line 2 column 2"},
	} {
		for _, name := range []string{"lookup", "tool_search", "  lookup  "} {
			t.Run(name+"/"+fixture.arguments, func(t *testing.T) {
				_, err := deepSeekResponseToolItem(map[string]any{
					"function": map[string]any{"name": name, "arguments": fixture.arguments},
				})
				want := fmt.Sprintf("DeepSeek returned malformed JSON arguments for tool call `%s`: %s", name, fixture.diagnostic)
				if err == nil || err.Error() != want {
					t.Fatalf("tool diagnostic = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestProdex04384BufferedToolObjectFailureRetainsEarlierOutput(t *testing.T) {
	const body = `{"id":"chatcmpl-object-error","created":1,"model":"deepseek-chat","choices":[{"message":{"content":"before error","tool_calls":[{"id":"call-valid","function":{"name":"lookup","arguments":"{}"}},{"id":"call-invalid","function":{"name":"lookup","arguments":"{bad"}},{"id":"call-later","function":{"name":"lookup","arguments":"{}"}}]}}]}`
	translated, err := deepSeekChatResponse([]byte(body), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Status string `json:"status"`
		Output []struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
		} `json:"output"`
		Error struct {
			Code, Message string
		} `json:"error"`
	}
	if err := json.Unmarshal(translated, &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "failed" || response.Error.Code != "invalid_tool_call_arguments" ||
		response.Error.Message != "DeepSeek returned malformed JSON arguments for tool call `lookup`: key must be a string at line 1 column 2" ||
		len(response.Output) != 2 || response.Output[0].Type != "message" || response.Output[1].CallID != "call-valid" {
		t.Fatalf("failed response lost earlier output or retained later tools: %s", translated)
	}
}

func TestProdex04384BufferedToolObjectSyntaxKeepsValidArguments(t *testing.T) {
	for _, name := range []string{"lookup", "tool_search"} {
		for _, arguments := range []string{`{}`, `{"key":"alpha"}`, `{"text":"{bad"}`, `[1,2]`, `null`} {
			if _, err := deepSeekResponseToolItem(map[string]any{
				"function": map[string]any{"name": name, "arguments": arguments},
			}); err != nil {
				t.Fatalf("valid %s arguments rejected: %v", name, err)
			}
		}
	}
}
