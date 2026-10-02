package chatcompat

import (
	"encoding/json"
	"errors"
	"strings"
)

func responseInputMessages(object map[string]any) ([]any, error) {
	messages := make([]any, 0)
	if instructions, ok := object["instructions"].(string); ok && instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	input, exists := object["input"]
	if !exists || input == nil {
		return nil, errors.New("Responses request must include a textual input or messages array")
	}
	switch value := input.(type) {
	case string:
		messages = append(messages, map[string]any{"role": "user", "content": value})
	case []any:
		converted, err := convertResponseItems(value)
		if err != nil {
			return nil, err
		}
		messages = append(messages, converted...)
	default:
		return nil, errors.New("Responses request must include a textual input or messages array")
	}
	if len(messages) == 0 || (len(messages) == 1 && messages[0].(map[string]any)["role"] == "system") {
		return nil, errors.New("Responses request must include a textual input or messages array")
	}
	return messages, nil
}

func convertResponseItems(items []any) ([]any, error) {
	result := make([]any, 0, len(items))
	var looseText []string
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		converted, text, flush, err := convertResponseItem(item)
		if err != nil {
			return nil, err
		}
		if flush {
			result = appendLooseText(result, &looseText)
		}
		result = append(result, converted...)
		if text != "" {
			looseText = append(looseText, text)
		}
	}
	return appendLooseText(result, &looseText), nil
}

func convertResponseItem(item map[string]any) ([]any, string, bool, error) {
	kind, _ := item["type"].(string)
	switch kind {
	case "message":
		message, err := convertMessageItem(item)
		if message == nil {
			return nil, "", true, err
		}
		return []any{message}, "", true, err
	case "function_call":
		return []any{convertFunctionCall(item)}, "", true, nil
	case "function_call_output":
		return []any{convertFunctionOutput(item)}, "", true, nil
	case "custom_tool_call":
		message, err := convertCustomToolCall(item)
		return oneMessage(message), "", true, err
	case "mcp_call":
		messages, err := convertMCPCall(item)
		return messages, "", true, err
	case "local_shell_call":
		message, err := convertLocalShellCall(item)
		return oneMessage(message), "", true, err
	case "custom_tool_call_output", "mcp_tool_result", "mcp_call_output":
		message, err := convertBridgeToolOutput(item)
		return oneMessage(message), "", true, err
	case "input_image":
		return nil, "", false, errors.New("Responses chat-compat only translates message/function-call history items")
	case "input_text", "output_text":
		text, _ := item["text"].(string)
		return nil, text, false, nil
	default:
		text, _ := item["content"].(string)
		return nil, text, false, nil
	}
}

func oneMessage(message map[string]any) []any {
	if message == nil {
		return nil
	}
	return []any{message}
}

func appendLooseText(result []any, looseText *[]string) []any {
	if len(*looseText) == 0 {
		return result
	}
	result = append(result, map[string]any{"role": "user", "content": strings.Join(*looseText, "\n")})
	*looseText = nil
	return result
}

func convertMessageItem(item map[string]any) (map[string]any, error) {
	role, _ := item["role"].(string)
	if role == "" {
		role = "user"
	}
	content, err := textContent(item["content"])
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, nil
	}
	return map[string]any{"role": role, "content": content}, nil
}

func textContent(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	parts, ok := value.([]any)
	if !ok {
		return "", nil
	}
	var texts []string
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := part["type"].(string); kind == "input_image" {
			return "", errors.New("Responses chat-compat currently translates only text input content")
		}
		if text, ok := part["text"].(string); ok && text != "" {
			texts = append(texts, text)
			continue
		}
		if text, ok := part["content"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func convertFunctionCall(item map[string]any) map[string]any {
	name, ok := item["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return nil
	}
	if namespace, ok := item["namespace"].(string); ok && namespace != "" {
		name = namespace + "." + name
	}
	callID, _ := item["call_id"].(string)
	arguments := compactJSONValue(item["arguments"])
	return map[string]any{
		"role": "assistant", "content": "",
		"tool_calls": []any{map[string]any{
			"id": callID, "type": "function",
			"function": map[string]any{"name": name, "arguments": arguments},
		}},
	}
}

func convertFunctionOutput(item map[string]any) map[string]any {
	callID, ok := item["call_id"].(string)
	if !ok || callID == "" {
		return nil
	}
	return map[string]any{"role": "tool", "tool_call_id": callID, "content": compactJSONValue(item["output"])}
}

func compactJSONValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	content, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(content)
}
