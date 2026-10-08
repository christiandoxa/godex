package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestDeepSeekResponsesToolShapingMatchesTaggedContract(t *testing.T) {
	for _, test := range []struct {
		name  string
		call  any
		check func(*testing.T, map[string]any)
	}{
		{
			name: "apply patch keeps arguments JSON string",
			call: map[string]any{"function": map[string]any{
				"name": "apply_patch", "arguments": `{"patch":"*** Begin Patch\\n*** End Patch"}`,
			}},
			check: func(t *testing.T, response map[string]any) {
				tool := deepSeekTestTool(t, response)
				if tool["type"] != "custom_tool_call" || tool["input"] != `{"patch":"*** Begin Patch\\n*** End Patch"}` {
					t.Fatalf("apply_patch output = %#v", tool)
				}
			},
		},
		{
			name: "separator priority and slash namespace",
			call: map[string]any{"function": map[string]any{
				"name": "scope/path.inner__lookup", "arguments": `{}`,
			}},
			check: func(t *testing.T, response map[string]any) {
				tool := deepSeekTestTool(t, response)
				if tool["namespace"] != "scope/path.inner" || tool["name"] != "lookup" {
					t.Fatalf("tagged separator priority = %#v", tool)
				}
			},
		},
		{
			name: "slash namespace",
			call: map[string]any{"function": map[string]any{
				"name": "scope/lookup", "arguments": `{}`,
			}},
			check: func(t *testing.T, response map[string]any) {
				tool := deepSeekTestTool(t, response)
				if tool["namespace"] != "scope" || tool["name"] != "lookup" {
					t.Fatalf("slash namespace = %#v", tool)
				}
			},
		},
		{
			name: "missing call id uses tagged fallback",
			call: map[string]any{"function": map[string]any{"name": "lookup", "arguments": `{}`}},
			check: func(t *testing.T, response map[string]any) {
				assertDeepSeekUUIDv7(t, deepSeekTestTool(t, response)["call_id"], "call_deepseek_")
			},
		},
		{
			name: "non-object tool call fails and keeps prior text",
			call: nil,
			check: func(t *testing.T, response map[string]any) {
				if response["status"] != "failed" || response["error"].(map[string]any)["code"] != "invalid_tool_call_arguments" {
					t.Fatalf("response error = %#v", response)
				}
				if output := response["output"].([]any); len(output) != 1 || output[0].(map[string]any)["type"] != "message" {
					t.Fatalf("partial output = %#v", output)
				}
			},
		},
		{
			name: "empty tool search arguments fail",
			call: map[string]any{"function": map[string]any{"name": "tool_search", "arguments": ""}},
			check: func(t *testing.T, response map[string]any) {
				if response["status"] != "failed" || !strings.Contains(response["error"].(map[string]any)["message"].(string), "tool_search") {
					t.Fatalf("empty tool_search response = %#v", response)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := map[string]any{"content": "before"}
			if test.call != nil {
				message["tool_calls"] = []any{test.call}
			} else {
				message["tool_calls"] = []any{nil}
			}
			response := executeDeepSeekBufferedResponse(t, map[string]any{
				"choices": []any{map[string]any{"message": message}},
			})
			test.check(t, response)
		})
	}
}

func TestDeepSeekResponsesRejectsOversizedToolNameOnRuntimePath(t *testing.T) {
	response := executeDeepSeekBufferedResponse(t, map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{
			"tool_calls": []any{map[string]any{"function": map[string]any{
				"name": strings.Repeat("x", deepSeekResponseNameMaxBytes+1), "arguments": `{}`,
			}}},
		}}},
	})
	if response["status"] != "failed" || !strings.Contains(response["error"].(map[string]any)["message"].(string), "name exceeding") {
		t.Fatalf("oversized tool name response = %#v", response)
	}
}

func TestDeepSeekResponseToolItemRejectsArgumentsOverTaggedLimit(t *testing.T) {
	_, err := deepSeekResponseToolItem(map[string]any{"function": map[string]any{
		"name": "lookup", "arguments": strings.Repeat("x", deepSeekResponseArgumentsMaxBytes+1),
	}})
	if err == nil || !strings.Contains(err.Error(), "arguments exceeding") {
		t.Fatalf("oversized arguments error = %v", err)
	}
}

func TestProdex04361DeepSeekRTKArgumentsRespectProviderBoundary(t *testing.T) {
	for _, test := range []struct {
		name       string
		valueBytes int
		failed     bool
	}{
		{name: "long environment assignment is wrapped", valueBytes: 15 << 20},
		{name: "scanner-sized assignment exceeds DeepSeek response limit", valueBytes: 17 << 20, failed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := "CACHE=" + strings.Repeat("x", test.valueBytes) + " cargo test"
			arguments, err := json.Marshal(map[string]string{"cmd": command})
			if err != nil {
				t.Fatal(err)
			}
			response := executeDeepSeekBufferedResponse(t, map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{
					"tool_calls": []any{map[string]any{"function": map[string]any{
						"name": "shell", "arguments": string(arguments),
					}}},
				}}},
			})
			if test.failed {
				if response["status"] != "failed" || !strings.Contains(response["error"].(map[string]any)["message"].(string), "arguments exceeding") {
					t.Fatalf("oversized RTK response = %#v", response)
				}
				return
			}
			tool := deepSeekTestTool(t, response)
			wrapped := strings.Replace(command, " cargo test", " rtk cargo test", 1)
			if tool["arguments"] != `{"cmd":"`+wrapped+`"}` {
				t.Fatalf("wrapped long RTK arguments = %#v", tool["arguments"])
			}
		})
	}
}

func TestDeepSeekResponseDefaultsUseTaggedTimeAndToolID(t *testing.T) {
	translated, err := deepSeekChatResponse([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"lookup","arguments":"{}"}}]}}]}`), time.Unix(42, 0))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(translated, &response); err != nil {
		t.Fatal(err)
	}
	assertDeepSeekUUIDv7(t, response["id"], "resp_deepseek_")
	assertDeepSeekUUIDv7(t, deepSeekTestTool(t, response)["call_id"], "call_deepseek_")
	if response["model"] != "deepseek-chat" || response["created_at"] != float64(42) {
		t.Fatalf("DeepSeek response defaults = %#v", response)
	}
}

func executeDeepSeekBufferedResponse(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(body)
	}))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "fixture-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Execute(context.Background(), proxymodel.Request{
		Method: http.MethodPost, Path: mountPath + "/responses", Body: []byte(`{"input":"test"}`),
	}, proxymodel.Account{})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	translated, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(translated, &result); err != nil {
		t.Fatalf("translated response %q: %v", translated, err)
	}
	return result
}

func deepSeekTestTool(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	output, ok := response["output"].([]any)
	if !ok || len(output) == 0 {
		t.Fatalf("response output = %#v", response["output"])
	}
	for _, raw := range output {
		if tool, ok := raw.(map[string]any); ok && tool["type"] != "message" {
			return tool
		}
	}
	t.Fatalf("response has no tool item: %#v", output)
	return nil
}
