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
		{hasKey(object, "messages"), "Responses chat-compat expects Responses input, not raw chat-completions messages"},
		{hasKey(object, "response_format"), "Responses chat-compat does not translate response_format controls"},
		{hasKey(object, "reasoning"), "Responses chat-compat does not map Responses reasoning controls"},
		{hasKey(object, "previous_response_id"), "Responses chat-compat does not map previous_response_id continuation state"},
		{hasTextFormat(object), "Responses chat-compat does not translate text.format controls"},
		{integerGreaterThanOne(object["n"]), "Responses chat-compat returns only the first choice and does not support n>1"},
		{hasKey(object, "metadata"), "Responses chat-compat does not translate request metadata"},
		{hasKey(object, "safety_identifier"), "Responses chat-compat does not translate safety_identifier"},
		{hasKey(object, "web_search_options"), "Responses chat-compat does not translate web_search_options"},
		{invalidTools(object["tools"]), "Responses chat-compat only forwards function tools"},
		{invalidToolChoice(object["tool_choice"]), "Responses chat-compat only forwards function tool_choice controls"},
		{falseBool(object["parallel_tool_calls"]), "Responses chat-compat does not prove a compatible parallel_tool_calls=false control"},
		{hasKey(object, "logprobs") || hasKey(object, "top_logprobs"), "Responses chat-compat does not translate logprobs controls"},
		{hasKey(object, "stop_sequences"), "Responses chat-compat does not translate stop_sequences"},
	}
	for _, check := range checks {
		if check.present {
			return errors.New(check.reason)
		}
	}
	return nil
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
