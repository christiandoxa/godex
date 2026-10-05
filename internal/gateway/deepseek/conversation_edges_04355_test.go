package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04355DeepSeekReplayEdgeCases(t *testing.T) {
	t.Run("duplicate function call is not replayed twice", func(t *testing.T) {
		store := newDeepSeekConversationStore().scoped("gateway")
		store.insert("chatcmpl_1", []any{
			map[string]any{"role": "user", "content": "read commit history"},
			deepSeekEdgeAssistant("call_1", "shell", `{"cmd":"git log --oneline -3"}`, "Need recent commits."),
		})
		body := translateDeepSeekEdgeRequest(t, store, `{
			"model":"deepseek-v4-pro",
			"previous_response_id":"chatcmpl_1",
			"input":[
				{"type":"function_call","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"git log --oneline -3\"}"},
				{"type":"function_call_output","call_id":"call_1","output":"a6f12b1 release"}
			]
		}`, false)
		messages := body["messages"].([]any)
		if len(messages) != 3 || messages[1].(map[string]any)["role"] != "assistant" ||
			len(messages[1].(map[string]any)["tool_calls"].([]any)) != 1 ||
			messages[2].(map[string]any)["role"] != "tool" {
			t.Fatalf("duplicate-call replay = %#v", messages)
		}
	})

	t.Run("function call preserves Gemini thought signature", func(t *testing.T) {
		store := newDeepSeekConversationStore().scoped("gateway")
		body := translateDeepSeekEdgeRequest(t, store, `{
			"model":"gemini-3.1-pro-preview",
			"input":[
				{"type":"function_call","call_id":"call_sig_1","name":"shell","arguments":"{\"cmd\":\"ls\"}","gemini_thought_signature":"sig-replay-1"},
				{"type":"function_call_output","call_id":"call_sig_1","output":"README.md"}
			]
		}`, false)
		messages := body["messages"].([]any)
		assistant := messages[0].(map[string]any)
		call := assistant["tool_calls"].([]any)[0].(map[string]any)
		if call["gemini_thought_signature"] != "sig-replay-1" {
			t.Fatalf("thought signature = %#v", call)
		}
	})

	t.Run("thinking tool turn repairs missing content and reasoning", func(t *testing.T) {
		store := newDeepSeekConversationStore().scoped("gateway")
		store.insert("chatcmpl_1", []any{
			map[string]any{"role": "user", "content": "read commit history"},
			deepSeekEdgeAssistant("call_1", "shell", `{"cmd":"git log --oneline -3"}`, ""),
		})
		body := translateDeepSeekEdgeRequest(t, store, `{
			"model":"deepseek-v4-pro",
			"previous_response_id":"chatcmpl_1",
			"input":[{"type":"function_call_output","call_id":"call_1","output":"f05b28c release"}],
			"reasoning":{"effort":"xhigh"}
		}`, true)
		messages := body["messages"].([]any)
		assistant := messages[1].(map[string]any)
		if assistant["content"] != "" || assistant["reasoning_content"] != "" {
			t.Fatalf("thinking assistant = %#v", assistant)
		}
	})

	t.Run("unanswered parallel calls are trimmed", func(t *testing.T) {
		store := newDeepSeekConversationStore().scoped("gateway")
		store.insert("chatcmpl_1", []any{
			map[string]any{"role": "user", "content": "inspect repo"},
			map[string]any{
				"role": "assistant", "content": nil, "reasoning_content": "Need two reads.",
				"tool_calls": []any{
					deepSeekEdgeCall("call_1", "shell", `{"cmd":"git status --short"}`),
					deepSeekEdgeCall("call_2", "shell", `{"cmd":"cat package.json"}`),
				},
			},
		})
		body := translateDeepSeekEdgeRequest(t, store, `{
			"model":"deepseek-v4-pro",
			"previous_response_id":"chatcmpl_1",
			"input":[{"type":"function_call_output","call_id":"call_1","output":"M file"}]
		}`, false)
		messages := body["messages"].([]any)
		assistant := messages[1].(map[string]any)
		calls := assistant["tool_calls"].([]any)
		encoded, _ := json.Marshal(messages)
		if len(messages) != 3 || len(calls) != 1 || calls[0].(map[string]any)["id"] != "call_1" ||
			containsString(string(encoded), "call_2") {
			t.Fatalf("parallel trim = %s", encoded)
		}
	})

	t.Run("late tool output moves adjacent to its call", func(t *testing.T) {
		store := newDeepSeekConversationStore().scoped("gateway")
		store.insert("chatcmpl_1", []any{
			map[string]any{"role": "user", "content": "read commit history"},
			deepSeekEdgeAssistant("call_1", "shell", `{"cmd":"git log --oneline -3"}`, "Need recent commits."),
		})
		body := translateDeepSeekEdgeRequest(t, store, `{
			"model":"deepseek-v4-pro",
			"previous_response_id":"chatcmpl_1",
			"input":[
				{"type":"message","role":"user","content":[{"type":"input_text","text":"continue after reading it"}]},
				{"type":"function_call_output","call_id":"call_1","output":"7828df3 fix"}
			]
		}`, false)
		messages := body["messages"].([]any)
		roles := deepSeekEdgeRoles(messages)
		if len(messages) != 4 || roles != "user,assistant,tool,user" ||
			messages[3].(map[string]any)["content"] != "continue after reading it" {
			t.Fatalf("late output replay = %#v", messages)
		}
	})

	t.Run("duplicate output after final answer is skipped", func(t *testing.T) {
		store := newDeepSeekConversationStore().scoped("gateway")
		store.insert("chatcmpl_2", []any{
			map[string]any{"role": "user", "content": "read commit history"},
			deepSeekEdgeAssistant("call_1", "shell", `{"cmd":"git log --oneline -3"}`, "Need recent commits."),
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "335a5dd release"},
			map[string]any{"role": "assistant", "content": "335a5dd release"},
		})
		body := translateDeepSeekEdgeRequest(t, store, `{
			"model":"deepseek-v4-pro",
			"previous_response_id":"chatcmpl_2",
			"input":[
				{"type":"function_call_output","call_id":"call_1","output":"335a5dd release"},
				{"type":"message","role":"user","content":[{"type":"input_text","text":"what else can be improved?"}]}
			]
		}`, false)
		messages := body["messages"].([]any)
		if len(messages) != 5 || deepSeekEdgeRoles(messages) != "user,assistant,tool,assistant,user" {
			t.Fatalf("final-answer replay = %#v", messages)
		}
		toolCount := 0
		for _, raw := range messages {
			message := raw.(map[string]any)
			if message["role"] == "tool" {
				toolCount++
			}
		}
		if toolCount != 1 {
			t.Fatalf("tool output count = %d in %#v", toolCount, messages)
		}
	})
}

func translateDeepSeekEdgeRequest(t *testing.T, store deepSeekConversationStore, body string, thinking bool) map[string]any {
	t.Helper()
	history := deepSeekConversationHistoryForRequest([]byte(body), store)
	translated, err := translateResponsesRequestWithHistory([]byte(body), RequestOptions{Model: "deepseek-v4-pro"}, history)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(translated.Body, &value); err != nil {
		t.Fatal(err)
	}
	if thinking {
		if enabled, _ := value["thinking"].(map[string]any); enabled["type"] != "enabled" {
			t.Fatalf("thinking was not enabled: %#v", value)
		}
	}
	return value
}

func deepSeekEdgeAssistant(callID, name, arguments, reasoning string) map[string]any {
	message := map[string]any{
		"role": "assistant", "content": nil,
		"tool_calls": []any{deepSeekEdgeCall(callID, name, arguments)},
	}
	if reasoning != "" {
		message["reasoning_content"] = reasoning
	}
	return message
}

func deepSeekEdgeCall(callID, name, arguments string) map[string]any {
	return map[string]any{
		"id": callID, "type": "function",
		"function": map[string]any{"name": name, "arguments": arguments},
	}
}

func deepSeekEdgeRoles(messages []any) string {
	roles := make([]string, 0, len(messages))
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		roles = append(roles, role)
	}
	return joinStrings(roles, ",")
}

func containsString(value, needle string) bool {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}

func joinStrings(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += separator + value
	}
	return result
}
