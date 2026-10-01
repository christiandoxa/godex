package chatcompat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func ResponsesRequest(body []byte, defaultModel, inputModel string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("failed to parse Responses request JSON: %w", err)
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil, errors.New("Responses request body must be a JSON object")
	}
	if err := rejectUnsupportedRequest(object); err != nil {
		return nil, err
	}
	messages, err := responseInputMessages(object)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any)
	result["model"] = responseModel(object, defaultModel, inputModel)
	result["messages"] = messages
	result["stream"] = boolValue(object["stream"])
	copyChatControls(result, object)
	if value, ok := chatMaxTokens(object); ok {
		result["max_tokens"] = value
	}
	return json.Marshal(result)
}

func rejectUnsupportedRequest(object map[string]any) error {
	checks := []struct {
		present bool
		reason  string
	}{
		{hasKey(object, "messages"), "anthropic Responses chat-compat expects Responses input, not raw chat-completions messages"},
		{hasKey(object, "response_format"), "anthropic Responses chat-compat does not translate response_format controls"},
		{hasKey(object, "reasoning"), "anthropic Responses chat-compat does not map Responses reasoning controls"},
		{hasKey(object, "previous_response_id"), "anthropic Responses chat-compat does not map previous_response_id continuation state"},
		{hasTextFormat(object), "anthropic Responses chat-compat does not translate text.format controls"},
		{integerGreaterThanOne(object["n"]), "anthropic Responses chat-compat returns only the first choice and does not support n>1"},
		{hasKey(object, "metadata"), "anthropic Responses chat-compat does not translate request metadata"},
		{hasKey(object, "safety_identifier"), "anthropic Responses chat-compat does not translate safety_identifier"},
		{hasKey(object, "web_search_options"), "anthropic Responses chat-compat does not translate web_search_options"},
		{invalidTools(object["tools"]), "anthropic Responses chat-compat only forwards function tools"},
		{invalidToolChoice(object["tool_choice"]), "anthropic Responses chat-compat only forwards function tool_choice controls"},
		{falseBool(object["parallel_tool_calls"]), "anthropic Responses chat-compat does not prove a compatible parallel_tool_calls=false control"},
		{hasKey(object, "logprobs") || hasKey(object, "top_logprobs"), "anthropic Responses chat-compat does not translate logprobs controls"},
		{hasKey(object, "stop_sequences"), "anthropic Responses chat-compat does not translate stop_sequences"},
	}
	for _, check := range checks {
		if check.present {
			return errors.New(check.reason)
		}
	}
	return nil
}

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
		if converted != nil {
			result = append(result, converted)
		}
		if text != "" {
			looseText = append(looseText, text)
		}
	}
	return appendLooseText(result, &looseText), nil
}

func convertResponseItem(item map[string]any) (any, string, bool, error) {
	kind, _ := item["type"].(string)
	switch kind {
	case "message":
		message, err := convertMessageItem(item)
		return message, "", true, err
	case "function_call":
		return convertFunctionCall(item), "", true, nil
	case "function_call_output":
		return convertFunctionOutput(item), "", true, nil
	case "input_image", "custom_tool_call":
		return nil, "", false, errors.New("anthropic Responses chat-compat only translates message/function-call history items")
	case "input_text", "output_text":
		text, _ := item["text"].(string)
		return nil, text, false, nil
	default:
		text, _ := item["content"].(string)
		return nil, text, false, nil
	}
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
			return "", errors.New("anthropic Responses chat-compat currently translates only text input content")
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

func responseModel(object map[string]any, fallback, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if model, ok := object["model"].(string); ok && strings.TrimSpace(model) != "" {
		return strings.TrimSpace(model)
	}
	return fallback
}

func copyChatControls(target, source map[string]any) {
	for _, key := range []string{"temperature", "top_p", "presence_penalty", "frequency_penalty", "seed", "tools", "tool_choice", "parallel_tool_calls", "user"} {
		if value, ok := source[key]; ok {
			target[key] = value
		}
	}
}

func chatMaxTokens(object map[string]any) (any, bool) {
	for _, key := range []string{"max_completion_tokens", "max_output_tokens", "max_tokens"} {
		if value, ok := object[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func hasKey(object map[string]any, key string) bool { _, ok := object[key]; return ok }
func hasTextFormat(object map[string]any) bool {
	text, ok := object["text"].(map[string]any)
	return ok && hasKey(text, "format")
}
func boolValue(value any) bool { result, _ := value.(bool); return result }
func falseBool(value any) bool { result, ok := value.(bool); return ok && !result }
func integerGreaterThanOne(value any) bool {
	number, ok := value.(json.Number)
	if !ok || strings.ContainsAny(number.String(), ".eE") {
		return false
	}
	parsed, err := number.Int64()
	return err == nil && parsed > 1
}
func invalidTools(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if ok {
			if kind, _ := item["type"].(string); kind != "" && kind != "function" {
				return true
			}
		}
	}
	return false
}
func invalidToolChoice(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	kind, _ := object["type"].(string)
	return kind != "" && kind != "function"
}
