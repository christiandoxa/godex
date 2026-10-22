package deepseek

import (
	"errors"
	"strings"
)

func deepSeekToolOutputItem(item map[string]any) (map[string]any, error) {
	callID := firstStringValue(item, deepSeekCallIDKey, deepSeekToolCallIDKey, "id")
	if strings.TrimSpace(callID) == "" {
		return nil, errors.New(deepSeekToolCallIDRequired)
	}
	value, ok := firstExistingValue(item, "output", deepSeekContentKey, "result", "error")
	if !ok {
		return nil, errors.New("DeepSeek input tool output items require output content")
	}
	content := value
	if item["type"] != "mcp_call" {
		content = deepSeekToolOutputText(value)
	}
	return map[string]any{"role": "tool", deepSeekToolCallIDKey: callID, deepSeekContentKey: content}, nil
}

func deepSeekToolOutputText(value any) string {
	values, ok := value.([]any)
	if !ok {
		return compactValue(value)
	}
	var parts []string
	for _, raw := range values {
		switch part := raw.(type) {
		case string:
			parts = append(parts, part)
		case map[string]any:
			for _, key := range []string{"text", "input_text", "output_text"} {
				if text, ok := part[key].(string); ok {
					parts = append(parts, text)
					break
				}
			}
		}
	}
	return strings.Join(parts, "\n")
}
