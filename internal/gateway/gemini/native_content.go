package gemini

import (
	"encoding/json"
	"strings"
)

func geminiMessageTexts(value any) []string {
	switch value := value.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []any:
		result := make([]string, 0, len(value))
		for _, raw := range value {
			switch part := raw.(type) {
			case string:
				if part != "" {
					result = append(result, part)
				}
			case map[string]any:
				if text, _ := part["text"].(string); text != "" {
					result = append(result, text)
				} else if text, _ := part["content"].(string); text != "" {
					result = append(result, text)
				}
			}
		}
		return result
	}
	return nil
}

func geminiMessageParts(value any) []any {
	if text, ok := value.(string); ok {
		if text == "" {
			return nil
		}
		return []any{map[string]any{"text": text}}
	}
	items, ok := value.([]any)
	if !ok {
		if object, ok := value.(map[string]any); ok {
			if text, ok := object["text"].(string); ok && text != "" {
				return []any{map[string]any{"text": text}}
			}
			if text, ok := object["content"].(string); ok && text != "" {
				return []any{map[string]any{"text": text}}
			}
		}
		return nil
	}
	parts := make([]any, 0, len(items))
	for _, raw := range items {
		switch part := raw.(type) {
		case string:
			if part != "" {
				parts = append(parts, map[string]any{"text": part})
			}
		case map[string]any:
			kind, _ := part["type"].(string)
			switch kind {
			case "input_text", "output_text":
				if text, ok := part["text"].(string); ok && text != "" {
					parts = append(parts, map[string]any{"text": text})
				}
			case "input_image":
				if media := geminiImagePart(part); media != nil {
					parts = append(parts, media)
				}
			case "input_audio":
				if data, ok := part["data"].(string); ok && data != "" {
					mime, _ := part["mime_type"].(string)
					parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": data}})
				}
			default:
				if text, ok := part["text"].(string); ok && text != "" {
					parts = append(parts, map[string]any{"text": text})
				} else if text, ok := part["content"].(string); ok && text != "" {
					parts = append(parts, map[string]any{"text": text})
				}
			}
		}
	}
	return parts
}

func geminiImagePart(part map[string]any) map[string]any {
	url, _ := part["image_url"].(string)
	if object, ok := part["image_url"].(map[string]any); ok {
		url, _ = object["url"].(string)
	}
	if url == "" {
		url, _ = part["url"].(string)
	}
	if mime, data, ok := geminiDataURL(url); ok {
		return map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": data}}
	}
	if url != "" {
		return map[string]any{"fileData": map[string]any{"fileUri": url, "mimeType": geminiImageMime(url)}}
	}
	return nil
}

func geminiImageMime(url string) string {
	return geminiMIMEForURI(url)
}

func geminiDataURL(value string) (string, string, bool) {
	rest, ok := strings.CutPrefix(value, "data:")
	if !ok {
		return "", "", false
	}
	metadata, data, ok := strings.Cut(rest, ",")
	if !ok {
		return "", "", false
	}
	base64Encoded := false
	for _, segment := range strings.Split(metadata, ";") {
		if strings.EqualFold(segment, "base64") {
			base64Encoded = true
			break
		}
	}
	if !base64Encoded {
		return "", "", false
	}
	mime := strings.SplitN(metadata, ";", 2)[0]
	if strings.TrimSpace(mime) == "" {
		mime = "application/octet-stream"
	}
	return mime, data, true
}

func geminiJoinSystemParts(parts []any) string {
	texts := make([]string, 0, len(parts))
	for _, raw := range parts {
		part, _ := raw.(map[string]any)
		text, _ := part["text"].(string)
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n\n")
}

func geminiIsContextualInstruction(text string) bool {
	trimmed := strings.TrimSpace(text)
	for _, prefix := range []string{"# AGENTS.md instructions for ", "<environment_context>", "<permissions instructions>", "<collaboration_mode>", "<skills_instructions>", "<plugins_instructions>", "<model_switch>", "<personality_spec>", "<realtime_conversation>"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func geminiJSONArguments(value any) any {
	switch value := value.(type) {
	case string:
		var parsed any
		if json.Unmarshal([]byte(value), &parsed) == nil && parsed != nil {
			return parsed
		}
		return map[string]any{}
	case nil:
		return map[string]any{}
	default:
		return value
	}
}

func geminiToolResponseValue(value any) any {
	text := ""
	switch value := value.(type) {
	case string:
		text = value
	case []any:
		if values := geminiMessageTexts(value); len(values) > 0 {
			text = strings.Join(values, "")
		}
	default:
		return map[string]any{"output": value}
	}
	var parsed any
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return parsed
	}
	return map[string]any{"output": text}
}
