package gemini

import (
	"reflect"
	"strings"
	"testing"
)

func TestProdex04380GeminiNativeMissingToolResultDoesNotExecuteOrInventSuccess(t *testing.T) {
	chat := map[string]any{"messages": []any{
		map[string]any{"role": "assistant", "content": "", "tool_calls": []any{
			map[string]any{"id": "call-missing", "function": map[string]any{"name": "lookup", "arguments": `{"q":"synthetic"}`}},
		}},
	}}
	payload, err := geminiGenerateContentRequest(chat, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	contents, ok := payload["contents"].([]any)
	if !ok || len(contents) != 2 {
		t.Fatalf("missing result was not inserted after single model call: %+v", payload)
	}
	first := contents[0].(map[string]any)
	last := contents[1].(map[string]any)
	call := first["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
	missing := last["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	if first["role"] != "model" || last["role"] != "user" || call["name"] != "lookup" ||
		missing["name"] != "lookup" || missing["id"] != "call-missing" ||
		!strings.Contains(missing["response"].(map[string]any)["error"].(string), "execution result was missing") {
		t.Fatalf("native missing result synthesized a success or changed identity: %+v", contents)
	}
}

func TestProdex04380GeminiNativeOrphanResultReceivesSyntheticNonExecutingCall(t *testing.T) {
	chat := map[string]any{"messages": []any{map[string]any{
		"role": "tool", "tool_call_id": "call-orphan", "content": `{"ok":true}`,
	}}}
	payload, err := geminiGenerateContentRequest(chat, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	contents, ok := payload["contents"].([]any)
	if !ok || len(contents) != 2 {
		t.Fatalf("unanswered native tool result was not repaired: %+v", payload)
	}
	synthesized := contents[0].(map[string]any)
	user := contents[1].(map[string]any)
	callPart := synthesized["parts"].([]any)[0].(map[string]any)
	call := callPart["functionCall"].(map[string]any)
	result := user["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	if synthesized["role"] != "model" || user["role"] != "user" ||
		call["id"] != "call-orphan" || call["name"] != "call-orphan" ||
		callPart["thoughtSignature"] != "skip_thought_signature_validator" ||
		!reflect.DeepEqual(call["args"], map[string]any{}) ||
		result["id"] != "call-orphan" {
		t.Fatalf("synthesized orphan call changed identity, signature or argument contract: %+v", contents)
	}
}

func TestProdex04380GeminiNativeMatchedToolPairDoesNotGetSyntheticDuplicate(t *testing.T) {
	chat := map[string]any{"messages": []any{
		map[string]any{"role": "assistant", "content": "", "tool_calls": []any{
			map[string]any{"id": "call-paired", "function": map[string]any{"name": "lookup", "arguments": "{}"}},
		}},
		map[string]any{"role": "tool", "tool_call_id": "call-paired", "content": "done"},
	}}
	payload, err := geminiGenerateContentRequest(chat, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	contents, ok := payload["contents"].([]any)
	if !ok || len(contents) != 2 {
		t.Fatalf("matched native tool pairing was duplicated: %+v", payload)
	}
	model := contents[0].(map[string]any)
	user := contents[1].(map[string]any)
	if len(model["parts"].([]any)) != 1 || len(user["parts"].([]any)) != 1 {
		t.Fatalf("paired native tool call or response was duplicated: %+v", contents)
	}
}

func TestProdex04380GeminiNativeReordersReversedToolResultsWithoutChangingIDs(t *testing.T) {
	chat := map[string]any{"messages": []any{
		map[string]any{"role": "assistant", "content": "", "tool_calls": []any{
			map[string]any{"id": "call-a", "function": map[string]any{"name": "lookup", "arguments": "{}"}},
			map[string]any{"id": "call-b", "function": map[string]any{"name": "lookup", "arguments": "{}"}},
		}},
		map[string]any{"role": "tool", "tool_call_id": "call-b", "content": "second"},
		map[string]any{"role": "tool", "tool_call_id": "call-a", "content": "first"},
	}}
	body, err := geminiGenerateContentRequest(chat, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	contents, ok := body["contents"].([]any)
	if !ok || len(contents) != 2 {
		t.Fatalf("reversed native tool results distorted contents: %+v", body)
	}
	model := contents[0].(map[string]any)
	user := contents[1].(map[string]any)
	modelParts := model["parts"].([]any)
	responses := user["parts"].([]any)
	if len(modelParts) != 2 || len(responses) != 2 || model["role"] != "model" || user["role"] != "user" {
		t.Fatalf("native tool pair cardinality or role changed: %+v", contents)
	}
	for index, expectedID := range []string{"call-a", "call-b"} {
		call := modelParts[index].(map[string]any)["functionCall"].(map[string]any)
		result := responses[index].(map[string]any)["functionResponse"].(map[string]any)
		if call["id"] != expectedID || result["id"] != expectedID || result["name"] != "lookup" {
			t.Fatalf("tool call/result ordering drift at %d: call=%+v result=%+v", index, call, result)
		}
	}
}
