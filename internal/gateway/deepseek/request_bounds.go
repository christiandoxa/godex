package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const requestPolicyMaxBytes = 4 << 20

func checkRequestPolicySize(object map[string]any) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return fmt.Errorf("serialize DeepSeek request policy input: %w", err)
	}
	// serde_json leaves U+2028/U+2029 literal; Go escapes them even without HTML escaping.
	size := encoded.Len() - 1 - deepSeekLineSeparatorEscapeBytes(object)
	if size > requestPolicyMaxBytes {
		return fmt.Errorf("DeepSeek request policy input exceeds %d bytes", requestPolicyMaxBytes)
	}
	return nil
}

func deepSeekLineSeparatorEscapeBytes(value any) int {
	size := 0
	switch value := value.(type) {
	case string:
		return 3 * (strings.Count(value, "\u2028") + strings.Count(value, "\u2029"))
	case map[string]any:
		for key, item := range value {
			size += deepSeekLineSeparatorEscapeBytes(key) + deepSeekLineSeparatorEscapeBytes(item)
		}
	case []any:
		for _, item := range value {
			size += deepSeekLineSeparatorEscapeBytes(item)
		}
	}
	return size
}
