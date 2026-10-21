package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestProdex04384GeminiStreamCanonicalRunShellAliases(t *testing.T) {
	for _, tc := range []struct{ input, name, arguments string }{
		{"run_shell_command", "exec_command", `{"cmd":"rtk git status"}`},
		{"RUN-SHELL-COMMAND", "exec_command", `{"cmd":"rtk git status"}`},
		{"\u2003ns.RUN-SHELL-COMMAND \u2003", "exec_command", `{"cmd":"rtk git status"}`},
		{"mcp__repo__run_shell_command", "run_shell_command", `{"cmd":"git status"}`},
		{"ns. run_shell_command", "ns. run_shell_command", `{"cmd":"git status"}`},
		{"ns.run-ſhell-command", "ns.run-ſhell-command", `{"cmd":"git status"}`},
	} {
		encoded, _ := json.Marshal(tc.input)
		wire := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":` + string(encoded) + `,"id":"call-alias","args":{"cmd":"git status"}}}]},"finishReason":"STOP"}]}`
		state := &geminiStreamState{responseID: "resp-alias"}
		var output bytes.Buffer
		if err := state.consume(&output, []byte(wire), nil); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, line := range strings.Split(output.String(), "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event map[string]any
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil || event["type"] != "response.completed" {
				continue
			}
			item := event["response"].(map[string]any)["output"].([]any)[0].(map[string]any)
			if item["name"] != tc.name || item["arguments"] != tc.arguments || item["call_id"] != "call-alias" {
				t.Errorf("canonical stream name %q result=%#v", tc.input, item)
			}
			found = true
		}
		if !found {
			t.Fatal("missing completed canonical stream tool")
		}
	}
}

func TestProdex04384GeminiStreamCompletedToolKinds(t *testing.T) {
	for _, tc := range []struct {
		name, args, kind string
		want             any
	}{
		{"shell", `{"cmd":"git status"}`, "function_call", `{"cmd":"rtk git status"}`},
		{"exec_command", `{"command":"git status"}`, "function_call", `{"command":"rtk git status"}`},
		{"exec", `{"cmd":"git status"}`, "function_call", `{"cmd":"git status"}`},
		{"namespace.shell", `{"cmd":"git status"}`, "function_call", `{"cmd":"git status"}`},
		{"mcp__tools__shell", `{"cmd":"git status"}`, "function_call", `{"cmd":"git status"}`},
		{"read_file", `null`, "function_call", `null`},
		{"tool_search", `{"query":"synthetic"}`, "tool_search_call", map[string]any{"query": "synthetic"}},
		{"tool_search", `null`, "tool_search_call", nil},
		{"apply_patch", `{"input":"--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+new"}`, "custom_tool_call", "*** Begin Patch\n*** Update File: file\n@@\n-old\n+new\n*** End Patch"},
		{"apply_patch", `{"input":"literal <tag>"}`, "custom_tool_call", "literal <tag>"},
	} {
		t.Run(tc.name+tc.args, func(t *testing.T) {
			for _, id := range []string{"call-stream", ""} {
				encodedName, _ := json.Marshal(tc.name)
				encodedID, _ := json.Marshal(id)
				chunk := `{"candidates":[{"content":{"parts":[{"functionCall":{"id":` + string(encodedID) + `,"name":` + string(encodedName) + `,"thoughtSignature":"sig-synthetic","args":` + tc.args + `}}]},"finishReason":"STOP"}]}`
				upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n"))}
				response, err := translateResponse(upstream, nil)
				if err != nil {
					t.Fatal(err)
				}
				wire, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				var item map[string]any
				var delta, deltaID string
				completed, deltas := 0, 0
				for _, line := range strings.Split(string(wire), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event map[string]any
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil {
						t.Fatalf("invalid SSE event: %s", line)
					}
					switch event["type"] {
					case "response.function_call_arguments.delta":
						deltas++
						delta, _ = event["delta"].(string)
						deltaID, _ = event["call_id"].(string)
					case "response.completed":
						output := event["response"].(map[string]any)["output"].([]any)
						if len(output) != 1 {
							t.Fatalf("stream changed tool count: %#v", output)
						}
						item = output[0].(map[string]any)
						completed++
					}
				}
				callID, _ := item["call_id"].(string)
				if completed != 1 || item["type"] != tc.kind || callID == "" || id != "" && callID != id {
					t.Fatalf("tool stream kind/delta/identity changed: item=%#v delta=%q id=%q completed=%d", item, delta, deltaID, completed)
				}
				if tc.kind == "function_call" {
					if deltas != 1 || delta != tc.args || callID != deltaID {
						t.Errorf("ordinary function delta changed: %s", wire)
					}
				} else if deltas != 0 {
					t.Errorf("special tool emitted forbidden function argument delta: %s", wire)
				}
				field := "arguments"
				if tc.kind == "custom_tool_call" {
					field = "input"
				}
				if !reflect.DeepEqual(item[field], tc.want) {
					t.Errorf("completed tool %s=%#v want=%#v", field, item[field], tc.want)
				}
				_, signature := item["gemini_thought_signature"]
				if signature != (tc.kind == "function_call") {
					t.Errorf("completed tool kind=%s signature presence=%v", tc.kind, signature)
				}
			}
		})
	}
}
