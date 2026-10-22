package deepseek

import (
	"errors"
	"fmt"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func deepSeekMessages(object map[string]any, preserveMessageCalls bool) ([]any, error) {
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
			converted, err := deepSeekInputItem(item, preserveMessageCalls)
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

func deepSeekInputItem(item map[string]any, preserveMessageCalls bool) ([]any, error) {
	if _, found := item["prefix"]; found {
		return nil, errors.New("DeepSeek chat prefix completion requires the beta chat endpoint, which is outside this Responses adapter")
	}
	kind, kindPresent := item["type"].(string)
	if _, found := item["type"]; found && (!kindPresent || kind == "") {
		return nil, errors.New("DeepSeek input item type `` is not supported by this Responses adapter")
	}
	switch kind {
	case "", "message":
		message, err := deepSeekMessageItem(item, preserveMessageCalls)
		if err != nil || message == nil {
			return nil, err
		}
		return []any{message}, nil
	case "function_call", "custom_tool_call", "mcp_call":
		messages, err := deepSeekToolCallItems(item, kind)
		if err != nil {
			return nil, err
		}
		return messages, nil
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

func deepSeekMessageItem(item map[string]any, preserveMessageCalls bool) (map[string]any, error) {
	role := "user"
	if value, ok := item["role"]; ok {
		text, valid := value.(string)
		if !valid {
			return nil, errors.New("DeepSeek message role must be a string")
		}
		if item["type"] == "message" && text != "user" && text != "assistant" && text != "system" && text != "developer" {
			return nil, fmt.Errorf("DeepSeek message role `%s` is not supported by this Responses adapter", text)
		}
		switch text {
		case "assistant", "system", "tool", "user":
			role = text
		case "developer":
			role = "system"
		}
	}
	contentValue, contentPresent := item[deepSeekContentKey]
	if item["type"] == "message" && contentPresent && contentValue == nil {
		return nil, errors.New("DeepSeek message content must be a string, object, or array")
	}
	content, err := deepSeekTextContent(contentValue)
	if err != nil {
		return nil, err
	}
	message := map[string]any{"role": role, deepSeekContentKey: content}
	if role == "tool" {
		callID := firstStringValue(item, deepSeekToolCallIDKey, deepSeekCallIDKey, "id")
		if strings.TrimSpace(callID) == "" {
			return nil, errors.New(deepSeekToolCallIDRequired)
		}
		message[deepSeekToolCallIDKey] = callID
	}
	if !preserveMessageCalls {
		return message, nil
	}
	toolCalls, err := deepSeekMessageToolCalls(item["tool_calls"])
	if err != nil {
		return nil, err
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}
	if strings.TrimSpace(content) == "" && len(toolCalls) == 0 {
		return nil, nil
	}
	return message, nil
}

func deepSeekMessageToolCalls(value any) ([]any, error) {
	if value == nil {
		return nil, nil
	}
	calls, ok := value.([]any)
	if !ok {
		return nil, errors.New("DeepSeek message tool_calls must be an array")
	}
	result := make([]any, 0, len(calls))
	for _, raw := range calls {
		call, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("DeepSeek message tool_calls entries must be objects")
		}
		id := firstStringValue(call, "id", deepSeekCallIDKey, deepSeekToolCallIDKey)
		function, ok := call[deepSeekFunctionKey].(map[string]any)
		if !ok {
			return nil, errors.New("DeepSeek message tool call must contain function")
		}
		name := firstStringValue(function, "name")
		if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
			return nil, errors.New("DeepSeek message tool call requires an id and function name")
		}
		arguments, present := function[deepSeekArgumentsKey]
		if !present {
			arguments = map[string]any{}
		}
		result = append(result, map[string]any{
			"id": id, "type": deepSeekFunctionKey,
			deepSeekFunctionKey: map[string]any{
				"name": name, deepSeekArgumentsKey: compactValue(arguments),
			},
		})
	}
	return result, nil
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

func deepSeekToolCallItems(item map[string]any, kind string) ([]any, error) {
	callID := firstStringValue(item, deepSeekCallIDKey, deepSeekToolCallIDKey, "id")
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New(deepSeekToolCallIDRequired)
	}
	name := firstStringValue(item, "name", "tool_name")
	_, topLevelNamePresent := item["name"].(string)
	if !topLevelNamePresent {
		_, topLevelNamePresent = item["tool_name"].(string)
	}
	if !topLevelNamePresent {
		if function, ok := item[deepSeekFunctionKey].(map[string]any); ok {
			name = firstStringValue(function, "name")
		}
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("DeepSeek input tool call items require a function name")
	}
	arguments := item[deepSeekArgumentsKey]
	switch kind {
	case "function_call", "mcp_call":
		arguments = deepSeekCallArguments04384(item)
	case "custom_tool_call":
		input := ""
		if value, present := item["input"]; present {
			input = compactValue(value)
		}
		arguments = map[string]any{"input": input}
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
	messages := []any{message}
	if kind == "mcp_call" {
		if _, found := firstExistingValue(item, "output", deepSeekContentKey, "result", "error"); found {
			output, err := deepSeekToolOutputItem(item)
			if err != nil {
				return nil, err
			}
			messages = append(messages, output)
		}
	}
	return messages, nil
}

func deepSeekCallArguments04384(item map[string]any) any {
	if value, present := firstExistingValue(item, deepSeekArgumentsKey, "input"); present {
		return value
	}
	function, _ := item[deepSeekFunctionKey].(map[string]any)
	if value, present := function[deepSeekArgumentsKey]; present {
		return value
	}
	return map[string]any{}
}

func deepSeekShellCallItem(item map[string]any) (map[string]any, error) {
	callID := firstStringValue(item, deepSeekCallIDKey, deepSeekToolCallIDKey, "id")
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New(deepSeekToolCallIDRequired)
	}
	command := ""
	if action, ok := item["action"].(map[string]any); ok {
		if raw, found := action["command"]; found {
			values, valid := raw.([]any)
			if !valid || len(values) == 0 {
				return nil, errors.New("DeepSeek local_shell_call action.command must be an array of strings")
			}
			parts := make([]string, 0, len(values))
			for _, value := range values {
				text, ok := value.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return nil, errors.New("DeepSeek local_shell_call action.command must be an array of strings")
				}
				parts = append(parts, text)
			}
			command = strings.Join(parts, " ")
		}
	} else {
		command = firstStringValue(item, "command")
	}
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("DeepSeek local_shell_call requires a command")
	}
	arguments := map[string]any{"command": command}
	for _, key := range []string{"cwd", "timeout", "env"} {
		if value, found := item[key]; found {
			arguments[key] = value
		}
	}
	return assistantToolCall(callID, "shell_command", arguments), nil
}

func assistantToolCall(callID, name string, arguments any) map[string]any {
	return map[string]any{
		"role": "assistant", deepSeekContentKey: "",
		"tool_calls": []any{map[string]any{
			"id": callID, "type": deepSeekFunctionKey,
			deepSeekFunctionKey: map[string]any{"name": name, deepSeekArgumentsKey: compactValue(arguments)},
		}},
	}
}

func compactValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	content, err := jsonutil.MarshalSerdeValue04384(value)
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

func firstExistingValue(object map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			return value, true
		}
	}
	return nil, false
}
