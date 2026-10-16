package deepseek

import (
	"encoding/json"
	"strings"
	"testing"
)

// Source: Prodex 0.438.0 at 8cab9794b0e08b2cb8507df8321a8e38ebd6492f,
// crates/prodex-provider-core/src/deepseek_bridge/tests/input_items.rs.
func TestProdex04380DeepSeekResponsesRejectsUnsupportedMessageRole(t *testing.T) {
	for _, role := range []string{"critic", "tool", "", "USER"} {
		t.Run(role, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"input": []any{map[string]any{
				"type": "message", "role": role, "content": "no", "tool_call_id": "call-fixture",
			}}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = ResponsesRequest(body, RequestOptions{})
			if err == nil || !strings.Contains(err.Error(), "DeepSeek message role `"+role+"` is not supported by this Responses adapter") {
				t.Fatalf("unsupported DeepSeek message role error = %v", err)
			}
		})
	}
}

// The tagged validator deliberately skips role validation without an explicit
// item type. Its separate chat-role shaper still defaults unknown roles to user.
func TestProdex04380DeepSeekResponsesUntypedRoleRetainsChatFallback(t *testing.T) {
	translated, err := ResponsesRequest([]byte(`{"input":[{"role":"critic","content":"note"}]}`), RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(translated, &body); err != nil {
		t.Fatal(err)
	}
	messages := body["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["role"] != "user" || messages[0].(map[string]any)["content"] != "note" {
		t.Fatalf("untyped chat-role fallback changed: %#v", messages)
	}
}

// Prodex 0.438.0's tagged input-item validator treats an explicit null
// content value as malformed; an omitted content field remains valid.
func TestProdex04380DeepSeekResponsesRejectsNullTypedMessageContent(t *testing.T) {
	_, err := ResponsesRequest([]byte(`{"input":[{"type":"message","role":"user","content":null}]}`), RequestOptions{})
	if err == nil || !strings.Contains(err.Error(), "DeepSeek message content must be a string, object, or array") {
		t.Fatalf("null typed message content error = %v", err)
	}
}

// The tagged Responses transform bounds the request object at the 4 MiB Mojo
// kernel limit; the separate SimpleRequest operation has a larger limit.
func TestProdex04380DeepSeekResponsesRejectsRequestOverKernelLimit(t *testing.T) {
	body := []byte(`{"input":"` + strings.Repeat("x", 4<<20) + `"}`)
	_, err := ResponsesRequest(body, RequestOptions{})
	if err == nil || !strings.Contains(err.Error(), "DeepSeek request policy input exceeds 4194304 bytes") {
		t.Fatalf("oversized Responses request error = %v", err)
	}
}

func TestProdex04380DeepSeekResponsesRejectsMalformedPresentInputType(t *testing.T) {
	for _, rawType := range []string{"null", "1", `""`} {
		t.Run(rawType, func(t *testing.T) {
			body := []byte(`{"input":[{"type":` + rawType + `,"content":"note"}]}`)
			_, err := ResponsesRequest(body, RequestOptions{})
			if err == nil || !strings.Contains(err.Error(), "DeepSeek input item type `` is not supported by this Responses adapter") {
				t.Fatalf("malformed input type %s error = %v", rawType, err)
			}
		})
	}
}

func TestProdex04380DeepSeekResponsesRejectsEmptyTopLevelToolName(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call","call_id":"call_1","name":"","function":{"name":"lookup"}}]}`)
	_, err := ResponsesRequest(body, RequestOptions{})
	if err == nil || !strings.Contains(err.Error(), "DeepSeek input tool call items require a function name") {
		t.Fatalf("empty top-level tool name error = %v", err)
	}
}

func TestProdex04380DeepSeekResponsesRejectsInvalidNestedShellCommand(t *testing.T) {
	body := []byte(`{"input":[{"type":"local_shell_call","call_id":"call_1","action":{"command":[]},"command":"fallback"}]}`)
	_, err := ResponsesRequest(body, RequestOptions{})
	if err == nil || !strings.Contains(err.Error(), "DeepSeek local_shell_call action.command must be an array of strings") {
		t.Fatalf("invalid nested shell command error = %v", err)
	}
}

func TestProdex04380DeepSeekResponsesRejectsBlankShellCallID(t *testing.T) {
	body := []byte(`{"input":[{"type":"local_shell_call","call_id":"  ","action":{"command":["echo","ok"]}}]}`)
	_, err := ResponsesRequest(body, RequestOptions{})
	if err == nil || !strings.Contains(err.Error(), "DeepSeek input tool items require a call_id") {
		t.Fatalf("blank shell call ID error = %v", err)
	}
}
