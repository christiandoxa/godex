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
		content = deepSeekBridgeContentText(value)
	}
	return map[string]any{"role": "tool", deepSeekToolCallIDKey: callID, deepSeekContentKey: content}, nil
}
