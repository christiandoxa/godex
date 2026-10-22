package chatcompat

import "strings"

// MergeHistory keeps cached Chat messages and deduplicates replayed input items.
func MergeHistory(history, current []any) []any {
	if len(history) == 0 {
		return current
	}
	result := CloneMessages(history)
	toolCalls := historyToolCallIDs(history)
	toolOutputs := historyToolOutputIDs(history)
	signatures := historyMessageSignatures(history)
	for _, raw := range current {
		message, _ := raw.(map[string]any)
		if message == nil {
			continue
		}
		role, _ := message["role"].(string)
		content, contentIsString := message["content"].(string)
		if contentIsString && strings.TrimSpace(content) != "" {
			if signatures[role+"\x00"+content] {
				continue
			}
		}
		if role == "tool" {
			callID, _ := message["tool_call_id"].(string)
			if callID != "" && toolOutputs[callID] {
				continue
			}
		}
		if role == "assistant" {
			calls, _ := message["tool_calls"].([]any)
			if len(calls) > 0 {
				filtered := make([]any, 0, len(calls))
				for _, rawCall := range calls {
					call, _ := rawCall.(map[string]any)
					id, _ := call["id"].(string)
					if id != "" && toolCalls[id] {
						continue
					}
					filtered = append(filtered, rawCall)
				}
				if len(filtered) == 0 && strings.TrimSpace(content) == "" {
					continue
				}
				copy := cloneHistoryMessage(message)
				copy["tool_calls"] = filtered
				raw = copy
			}
		}
		result = append(result, raw)
	}
	return result
}

func historyToolCallIDs(messages []any) map[string]bool {
	result := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, _ := rawCall.(map[string]any)
			if id, _ := call["id"].(string); strings.TrimSpace(id) != "" {
				result[id] = true
			}
		}
	}
	return result
}

func historyToolOutputIDs(messages []any) map[string]bool {
	result := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		if id, _ := message["tool_call_id"].(string); strings.TrimSpace(id) != "" {
			result[id] = true
		}
	}
	return result
}

func historyMessageSignatures(messages []any) map[string]bool {
	result := make(map[string]bool)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		role, _ := message["role"].(string)
		content, _ := message["content"].(string)
		if strings.TrimSpace(content) != "" {
			result[role+"\x00"+content] = true
		}
	}
	return result
}

func cloneHistoryMessage(message map[string]any) map[string]any {
	cloned := CloneMessages([]any{message})
	if len(cloned) != 1 {
		return map[string]any{}
	}
	result, _ := cloned[0].(map[string]any)
	return result
}
