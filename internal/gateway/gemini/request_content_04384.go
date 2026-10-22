package gemini

import (
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func validGeminiChatContent04384(value any) bool {
	switch content := value.(type) {
	case string:
		return true
	case map[string]any:
		return validGeminiChatContentPart04384(content)
	case []any:
		for _, raw := range content {
			if part, ok := raw.(map[string]any); ok && !validGeminiChatContentPart04384(part) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func validGeminiChatContentPart04384(part map[string]any) bool {
	if _, present := part["cache_control"]; present {
		return false
	}
	if _, selected := geminiChatTextPart04384(part); selected {
		return true
	}
	switch part["type"] {
	case "input_image", "image_url", "input_file", "file", "media", "input_audio", "input_video":
		return true
	default:
		return false
	}
}

func geminiChatContentText04384(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	if parts, ok := value.([]any); ok {
		texts := make([]string, 0, len(parts))
		for _, part := range parts {
			if text, selected := geminiChatTextPart04384(part); selected {
				texts = append(texts, text)
			}
		}
		return strings.Join(texts, "\n"), nil
	}
	encoded, err := jsonutil.MarshalSerdeValue04384(value)
	return string(encoded), err
}

func geminiChatTextPart04384(value any) (string, bool) {
	if text, ok := value.(string); ok {
		return text, true
	}
	if part, ok := value.(map[string]any); ok {
		for _, field := range []string{"text", "input_text", "output_text"} {
			if text, ok := part[field].(string); ok {
				return text, true
			}
		}
	}
	return "", false
}
