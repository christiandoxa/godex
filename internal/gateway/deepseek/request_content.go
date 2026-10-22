package deepseek

import "strings"

func deepSeekBridgeContentText(value any) string {
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
