package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestProdex04384GeminiToolNamesPreserveRawNamespacePolicy(t *testing.T) {
	for _, tc := range []struct{ flat, namespace, name string }{
		{"plain_tool", "", "plain_tool"},
		{"namespace--tool", "namespace", "tool"},
		{"mcp__repo__tool", "mcp__repo", "tool"},
		{"namespace.shell", "", "namespace.shell"},
		{"  read_file  ", "", "  read_file  "},
		{"  ns--tool  ", "  ns", "tool  "},
		{"mcp____tool", "mcp__", "tool"},
		{"namespace--sub--", "namespace", "sub--"},
		{"namespace--\u2003", "", "namespace--\u2003"},
		{"mcp__repo__tool--", "mcp__repo", "tool--"},
		{"mcp__repo__\u2003", "", "mcp__repo__\u2003"},
		{"日本語--🔥", "日本語", "🔥"},
		{"", "", ""},
		{"\u2003", "", "\u2003"},
	} {
		t.Run(tc.flat, func(t *testing.T) {
			call := map[string]any{"name": tc.flat, "id": "call-name", "args": map[string]any{}}
			part := map[string]any{"functionCall": call}
			geminiAssertToolName04384(t, geminiNativeToolCallItem(part, call, "fallback"), tc.namespace, tc.name)
			chunk := map[string]any{"candidates": []any{map[string]any{
				"content": map[string]any{"parts": []any{part}}, "finishReason": "STOP",
			}}}
			wire, err := json.Marshal(chunk)
			if err != nil {
				t.Fatal(err)
			}
			state := &geminiStreamState{responseID: "resp-tool-name"}
			var events bytes.Buffer
			if err := state.consume(&events, wire, nil); err != nil {
				t.Fatal(err)
			}
			if err := state.consume(&events, []byte("[DONE]"), nil); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, line := range strings.Split(events.String(), "\n") {
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var event map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil || event["type"] != "response.completed" {
					continue
				}
				response := event["response"].(map[string]any)
				output := response["output"].([]any)
				if len(output) != 1 {
					t.Fatalf("stream lost tool call: %#v", response)
				}
				geminiAssertToolName04384(t, output[0].(map[string]any), tc.namespace, tc.name)
				found = true
			}
			if !found {
				t.Fatalf("stream omitted completed tool: %s", events.String())
			}
		})
	}
}

func TestProdex04384GeminiBufferedToolStringPresence(t *testing.T) {
	for _, tc := range []struct {
		name             any
		id               any
		wantName, wantID string
	}{
		{nil, nil, "tool_call", "fallback"},
		{7, 7, "tool_call", "fallback"},
		{"", "", "", ""},
		{" ", "\u2003", " ", "\u2003"},
		{"read_file", "call-normal", "read_file", "call-normal"},
	} {
		call := map[string]any{"name": tc.name, "id": tc.id}
		item := geminiNativeToolCallItem(nil, call, "fallback")
		if item["name"] != tc.wantName || item["call_id"] != tc.wantID {
			t.Errorf("typed buffered tool strings name=%#v id=%#v item=%#v", tc.name, tc.id, item)
		}
	}
}

func geminiAssertToolName04384(t *testing.T, item map[string]any, namespace, name string) {
	t.Helper()
	gotNamespace, present := item["namespace"]
	if item["type"] != "function_call" || item["name"] != name || present != (namespace != "") || present && gotNamespace != namespace {
		t.Errorf("tool shape=%#v want namespace=%q name=%q", item, namespace, name)
	}
}
