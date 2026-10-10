package gemini

import (
	"encoding/json"
	"strings"
)

func geminiInputMessages(request map[string]any) ([]any, error) {
	messages := make([]any, 0)
	if instructions, ok := request["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	input, exists := request["input"]
	if !exists || input == nil {
		return append(messages, map[string]any{"role": "user", "content": ""}), nil
	}
	if text, ok := input.(string); ok {
		return append(messages, map[string]any{"role": "user", "content": text}), nil
	}
	items, ok := input.([]any)
	if !ok {
		return append(messages, map[string]any{"role": "user", "content": ""}), nil
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			if text, ok := raw.(string); ok {
				messages = append(messages, map[string]any{"role": "user", "content": text})
			}
			continue
		}
		converted, ok := geminiInputMessage(item)
		if ok {
			messages = append(messages, converted...)
		}
	}
	if len(messages) == 0 {
		return []any{map[string]any{"role": "user", "content": ""}}, nil
	}
	return messages, nil
}

func geminiInputMessage(item map[string]any) ([]any, bool) {
	kind, _ := item["type"].(string)
	if kind == "function_call" {
		name, _ := item["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, false
		}
		callID, _ := item["call_id"].(string)
		return []any{map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{
			"id": callID, "type": "function", "function": map[string]any{"name": name, "arguments": geminiInputArguments(item["arguments"])},
		}}}}, true
	}
	if kind == "function_call_output" {
		callID, _ := item["call_id"].(string)
		if callID == "" {
			return nil, false
		}
		return []any{map[string]any{"role": "tool", "tool_call_id": callID, "content": geminiInputValueText(item["output"])}}, true
	}
	if kind == "message" {
		role, _ := item["role"].(string)
		if role == "" {
			role = "user"
		}
		message := map[string]any{"role": role, "content": item["content"]}
		if calls, ok := item["tool_calls"]; ok {
			message["tool_calls"] = calls
		}
		return []any{message}, true
	}
	if role, ok := item["role"].(string); ok && role != "" {
		content := item["content"]
		if _, ok := content.(string); !ok {
			if _, ok := content.([]any); !ok {
				content = item["text"]
			}
		}
		message := map[string]any{"role": role, "content": content}
		if calls, ok := item["tool_calls"]; ok {
			message["tool_calls"] = calls
		}
		for _, key := range []string{"tool_call_id", "name"} {
			if value, exists := item[key]; exists {
				message[key] = value
			}
		}
		return []any{message}, true
	}
	if kind == "input_text" || kind == "output_text" {
		if text, ok := item["text"].(string); ok {
			return []any{map[string]any{"role": "user", "content": text}}, true
		}
	}
	if text, ok := item["content"].(string); ok {
		return []any{map[string]any{"role": "user", "content": text}}, true
	}
	return nil, false
}

func geminiInputArguments(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func geminiInputValueText(value any) any {
	if text, ok := value.(string); ok {
		return text
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
