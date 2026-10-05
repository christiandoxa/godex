package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	chatcompat "github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func deepSeekMessages(object map[string]any) ([]any, error) {
	messages := make([]any, 0)
	if instructions, ok := object["instructions"].(string); ok && strings.TrimSpace(instructions) != "" {
		messages = append(messages, map[string]any{"role": "system", deepSeekContentKey: instructions})
	}
	input, exists := object["input"]
	if !exists || input == nil {
		return messages, nil
	}
	switch value := input.(type) {
	case string:
		messages = append(messages, map[string]any{"role": "user", deepSeekContentKey: value})
	case []any:
		for _, raw := range value {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("DeepSeek input items must be objects")
			}
			converted, err := deepSeekInputItem(item)
			if err != nil {
				return nil, err
			}
			messages = append(messages, converted...)
		}
	default:
		return nil, errors.New("DeepSeek input must be a string or array of input items")
	}
	return messages, nil
}

func deepSeekInputItem(item map[string]any) ([]any, error) {
	if _, found := item["prefix"]; found {
		return nil, errors.New("DeepSeek chat prefix completion requires the beta chat endpoint, which is outside this Responses adapter")
	}
	kind, _ := item["type"].(string)
	switch kind {
	case "", "message":
		message, err := deepSeekMessageItem(item)
		if err != nil || message == nil {
			return nil, err
		}
		return []any{message}, nil
	case "function_call", "custom_tool_call", "mcp_call":
		message, err := deepSeekToolCallItem(item, kind)
		if err != nil {
			return nil, err
		}
		return []any{message}, nil
	case "local_shell_call":
		message, err := deepSeekShellCallItem(item)
		if err != nil {
			return nil, err
		}
		return []any{message}, nil
	case "function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output":
		message, err := deepSeekToolOutputItem(item)
		if err != nil {
			return nil, err
		}
		return []any{message}, nil
	default:
		return nil, fmt.Errorf("DeepSeek input item type `%s` is not supported by this Responses adapter", kind)
	}
}

func deepSeekMessageItem(item map[string]any) (map[string]any, error) {
	role := "user"
	if value, ok := item["role"]; ok {
		text, valid := value.(string)
		if !valid {
			return nil, errors.New("DeepSeek message role must be a string")
		}
		switch text {
		case "assistant", "system", "tool":
			role = text
		case "developer":
			role = "system"
		default:
			role = "user"
		}
	}
	content, err := deepSeekTextContent(item[deepSeekContentKey])
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	return map[string]any{"role": role, deepSeekContentKey: content}, nil
}

func deepSeekTextContent(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case map[string]any:
		return deepSeekContentPart(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, raw := range typed {
			switch part := raw.(type) {
			case string:
				parts = append(parts, part)
			case map[string]any:
				text, err := deepSeekContentPart(part)
				if err != nil {
					return "", err
				}
				if text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", errors.New("DeepSeek message content must be a string, object, or array")
	}
}

func deepSeekContentPart(part map[string]any) (string, error) {
	if _, found := part["prefix"]; found {
		return "", errors.New("DeepSeek chat prefix completion requires the beta chat endpoint, which is outside this Responses adapter")
	}
	if _, found := part["cache_control"]; found {
		return "", errors.New("DeepSeek per-message cache_control is not supported by this Responses adapter because DeepSeek context caching is automatic")
	}
	for _, key := range []string{"text", "input_text", "output_text"} {
		if value, found := part[key]; found {
			if text, ok := value.(string); ok {
				return text, nil
			}
		}
	}
	kind, _ := part["type"].(string)
	if kind == "" {
		return "", errors.New("DeepSeek text-only adapter does not support object message content parts without text or type")
	}
	if kind == "input_text" || kind == "output_text" || kind == "text" {
		return "", fmt.Errorf("DeepSeek %s content parts require a text field", kind)
	}
	return "", fmt.Errorf("DeepSeek text-only adapter does not support message content part type `%s`", kind)
}

func deepSeekToolCallItem(item map[string]any, kind string) (map[string]any, error) {
	callID := firstStringValue(item, deepSeekCallIDKey, deepSeekToolCallIDKey, "id")
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New(deepSeekToolCallIDRequired)
	}
	name := firstStringValue(item, "name", "tool_name")
	if name == "" {
		if function, ok := item[deepSeekFunctionKey].(map[string]any); ok {
			name = firstStringValue(function, "name")
		}
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("DeepSeek input tool call items require a function name")
	}
	arguments := item[deepSeekArgumentsKey]
	if kind == "custom_tool_call" || kind == "mcp_call" {
		if arguments == nil {
			arguments = firstValue(item, "input", deepSeekArgumentsKey)
		}
	}
	message := assistantToolCall(callID, name, arguments)
	if signature, _ := item["gemini_thought_signature"].(string); strings.TrimSpace(signature) != "" {
		calls, _ := message["tool_calls"].([]any)
		if len(calls) > 0 {
			if call, ok := calls[0].(map[string]any); ok {
				call["gemini_thought_signature"] = signature
			}
		}
	}
	return message, nil
}

func deepSeekShellCallItem(item map[string]any) (map[string]any, error) {
	callID := firstStringValue(item, deepSeekCallIDKey, deepSeekToolCallIDKey, "id")
	if callID == "" {
		return nil, errors.New(deepSeekToolCallIDRequired)
	}
	command := ""
	if action, ok := item["action"].(map[string]any); ok {
		if raw, ok := action["command"].([]any); ok && len(raw) > 0 {
			parts := make([]string, 0, len(raw))
			for _, value := range raw {
				text, ok := value.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return nil, errors.New("DeepSeek local_shell_call action.command must be an array of strings")
				}
				parts = append(parts, text)
			}
			command = strings.Join(parts, " ")
		}
	}
	if command == "" {
		command = firstStringValue(item, "command")
	}
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("DeepSeek local_shell_call requires a command")
	}
	return assistantToolCall(callID, "exec_command", map[string]any{"cmd": command}), nil
}

func deepSeekToolOutputItem(item map[string]any) (map[string]any, error) {
	callID := firstStringValue(item, deepSeekCallIDKey, deepSeekToolCallIDKey, "id")
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New(deepSeekToolCallIDRequired)
	}
	value, ok := firstExistingValue(item, "output", deepSeekContentKey, "result", "error")
	if !ok {
		return nil, errors.New("DeepSeek input tool output items require output content")
	}
	return map[string]any{"role": "tool", deepSeekToolCallIDKey: callID, deepSeekContentKey: compactValue(value)}, nil
}

func assistantToolCall(callID, name string, arguments any) map[string]any {
	argumentText := chatcompat.WrapRTKArguments(name, compactValue(arguments))
	return map[string]any{
		"role": "assistant", deepSeekContentKey: "",
		"tool_calls": []any{map[string]any{
			"id": callID, "type": deepSeekFunctionKey,
			deepSeekFunctionKey: map[string]any{"name": name, deepSeekArgumentsKey: argumentText},
		}},
	}
}

func compactValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	content, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(content)
}

func firstStringValue(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok {
			return value
		}
	}
	return ""
}

func firstValue(object map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value
		}
	}
	return nil
}

func firstExistingValue(object map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value, true
		}
	}
	return nil, false
}
