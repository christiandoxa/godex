package chatcompat

import (
	"errors"
	"strings"
)

func convertCustomToolCall(item map[string]any) (map[string]any, error) {
	callID := responseItemCallID(item)
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New("Responses chat-compat input tool call items require a call_id")
	}
	name, _ := firstResponseString(item, "name", "tool_name")
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("Responses chat-compat input tool call items require a function name")
	}
	input, exists := item["input"]
	if !exists {
		input = ""
	}
	arguments := compactJSONValue(map[string]any{"input": compactJSONValue(input)})
	return toolCallMessage(callID, name, arguments), nil
}

func convertMCPCall(item map[string]any) ([]any, error) {
	callID := responseItemCallID(item)
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New("Responses chat-compat input tool call items require a call_id")
	}
	name, present := firstResponseString(item, "name", "tool_name")
	if !present {
		if function, ok := item["function"].(map[string]any); ok {
			name, _ = function["name"].(string)
		}
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("Responses chat-compat input tool call items require a function name")
	}
	arguments, exists := item["arguments"]
	if !exists {
		arguments, exists = item["input"]
	}
	if !exists {
		if function, ok := item["function"].(map[string]any); ok {
			arguments, exists = function["arguments"]
		}
	}
	if !exists {
		arguments = map[string]any{}
	}
	messages := []any{toolCallMessage(callID, name, compactJSONValue(arguments))}
	if output, exists := responseToolOutput(item); exists {
		messages = append(messages, map[string]any{"role": "tool", "tool_call_id": callID, "content": output})
	}
	return messages, nil
}

func convertLocalShellCall(item map[string]any) (map[string]any, error) {
	callID := responseItemCallID(item)
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New("Responses chat-compat input tool call items require a call_id")
	}
	action, _ := item["action"].(map[string]any)
	command, exists := item["command"]
	if !exists {
		command, exists = action["command"]
	}
	var commandText string
	switch value := command.(type) {
	case string:
		commandText = value
	case []any:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			text, ok := part.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, errors.New("Responses chat-compat local_shell_call action.command must be an array of strings")
			}
			parts = append(parts, text)
		}
		commandText = strings.Join(parts, " ")
	default:
		return nil, errors.New("Responses chat-compat local_shell_call requires a command")
	}
	if strings.TrimSpace(commandText) == "" {
		return nil, errors.New("Responses chat-compat local_shell_call requires a command")
	}
	arguments := map[string]any{"command": commandText}
	for _, key := range []string{"cwd", "timeout", "env"} {
		if value, exists := item[key]; exists {
			arguments[key] = value
		} else if value, exists := action[key]; exists {
			arguments[key] = value
		}
	}
	return toolCallMessage(callID, "shell_command", compactJSONValue(arguments)), nil
}

func convertBridgeToolOutput(item map[string]any) (map[string]any, error) {
	callID := responseItemCallID(item)
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New("Responses chat-compat input tool output items require a call_id")
	}
	output, exists := responseToolOutput(item)
	if !exists {
		return nil, errors.New("Responses chat-compat input tool output items require output content")
	}
	if parts, ok := output.([]any); ok {
		text, _ := textContent(parts)
		output = text
	} else if _, ok := output.(string); !ok {
		output = compactJSONValue(output)
	}
	return map[string]any{"role": "tool", "tool_call_id": callID, "content": output}, nil
}

func responseToolOutput(item map[string]any) (any, bool) {
	for _, key := range []string{"output", "content", "result", "error"} {
		if value, exists := item[key]; exists {
			return value, true
		}
	}
	return nil, false
}

func responseItemCallID(item map[string]any) string {
	callID, _ := firstResponseString(item, "call_id", "tool_call_id", "id")
	return callID
}

func firstResponseString(object map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, exists := object[key]; exists {
			text, _ := value.(string)
			return text, true
		}
	}
	return "", false
}

func toolCallMessage(callID, name, arguments string) map[string]any {
	return map[string]any{
		"role": "assistant", "content": "",
		"tool_calls": []any{map[string]any{
			"id": callID, "type": "function",
			"function": map[string]any{"name": name, "arguments": arguments},
		}},
	}
}
