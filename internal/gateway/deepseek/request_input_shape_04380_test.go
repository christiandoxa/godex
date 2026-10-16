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
