package deepseek

import (
	"errors"
	"fmt"
	"strings"
)

func deepSeekResponseFormat(object map[string]any) (any, bool, error) {
	value, found := object["response_format"]
	if !found {
		if text, ok := object["text"].(map[string]any); ok {
			value, found = text["format"]
		}
	}
	if !found {
		return nil, false, nil
	}
	format, ok := value.(map[string]any)
	if !ok {
		return nil, false, errors.New("DeepSeek response_format must be an object")
	}
	kind, ok := format["type"].(string)
	if !ok || strings.TrimSpace(kind) == "" {
		return nil, false, errors.New("DeepSeek response_format must include a type")
	}
	switch kind {
	case "text":
		return nil, false, nil
	case "json", "json_object", "json_schema", "structured_output":
		return map[string]any{"type": "json_object"}, true, nil
	default:
		return nil, false, fmt.Errorf("DeepSeek response_format type `%s` is not supported", kind)
	}
}

func ensureJSONInstruction(messages []any) []any {
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if content, ok := message["content"].(string); ok &&
			strings.Contains(strings.ToLower(content), "json") {
			return messages
		}
	}
	result := make([]any, 0, len(messages)+1)
	result = append(result, map[string]any{
		"role": "system", "content": "Respond with valid JSON only.",
	})
	result = append(result, messages...)
	return result
}
