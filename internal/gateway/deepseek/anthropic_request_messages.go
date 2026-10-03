package deepseek

import (
	"errors"
	"strings"
)

func anthropicChatMessages(value any) ([]any, string, error) {
	messages, system, err := parseAnthropicChatMessages(value)
	if err != nil {
		return nil, "", err
	}
	result := make([]any, 0, len(messages))
	for index := 0; index < len(messages); {
		next, role, group := anthropicMessageGroup(messages, index)
		index = next
		if len(group) == 0 {
			continue
		}
		blocks, err := anthropicMessageBlocks(group)
		if err != nil {
			return nil, "", err
		}
		if len(blocks) > 0 {
			result = append(result, map[string]any{"role": role, "content": blocks})
		}
	}
	if len(result) == 0 {
		return nil, "", errors.New("Responses request must contain at least one user or assistant message")
	}
	return result, anthropicSystemText(system), nil
}

func parseAnthropicChatMessages(value any) ([]map[string]any, []string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, nil, errors.New("translated Responses request must contain messages")
	}
	messages := make([]map[string]any, 0, len(items))
	var system []string
	for _, raw := range items {
		message, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, errors.New("translated message must be an object")
		}
		messages = append(messages, message)
		if anthropicMessageRole(message) == "system" {
			if content, ok := message["content"].(string); ok {
				system = append(system, content)
			}
		}
	}
	return messages, system, nil
}

func anthropicMessageGroup(messages []map[string]any, start int) (int, string, []map[string]any) {
	index := start
	for index < len(messages) && anthropicMessageRole(messages[index]) == "system" {
		index++
	}
	if index >= len(messages) {
		return index, "", nil
	}
	role := anthropicMessageRole(messages[index])
	group := make([]map[string]any, 0, 1)
	for index < len(messages) {
		current := messages[index]
		if anthropicMessageRole(current) == "system" {
			index++
			continue
		}
		if anthropicMessageRole(current) != role {
			break
		}
		group = append(group, current)
		index++
	}
	return index, role, group
}

func anthropicMessageBlocks(group []map[string]any) ([]any, error) {
	blocks := make([]any, 0)
	for _, current := range group {
		if current["role"] != "tool" {
			continue
		}
		blocks = append(blocks, map[string]any{
			"type":        "tool_result",
			"tool_use_id": stringOr(current["tool_call_id"], "call_prodex"),
			"content":     stringOr(current["content"], ""),
		})
	}
	for _, current := range group {
		if current["role"] == "tool" {
			continue
		}
		if content := stringOr(current["content"], ""); content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": content})
		}
		calls, _ := current["tool_calls"].([]any)
		for _, rawCall := range calls {
			block, err := anthropicToolUse(rawCall)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func anthropicSystemText(system []string) string {
	if len(system) == 1 && system[0] == "" {
		return ""
	}
	var result strings.Builder
	for index, text := range system {
		if index > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString(text)
	}
	return result.String()
}

func anthropicMessageRole(message map[string]any) string {
	switch message["role"] {
	case "system", "developer":
		return "system"
	case "assistant":
		return "assistant"
	default:
		return "user"
	}
}
