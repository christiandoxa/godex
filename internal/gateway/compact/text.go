package compact

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

func textFromContent(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		return textFromArray(typed)
	case map[string]any:
		return textFromObject(typed)
	case json.Number:
		return typed.String()
	case bool:
		return boolText(typed)
	default:
		return ""
	}
}

func textFromArray(values []any) string {
	parts := make([]string, 0, len(values))
	for _, item := range values {
		if text := textFromContent(item); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func textFromObject(object map[string]any) string {
	for _, key := range []string{"text", "output", "input", "query", "command", "commands"} {
		if text := textFromContent(object[key]); strings.TrimSpace(text) != "" {
			return text
		}
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if text := textFromContent(object[key]); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func truncateUTF8(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	const suffix = "\n[truncated]"
	if maxBytes <= 0 {
		return ""
	}
	if maxBytes <= len(suffix) {
		return suffix[:maxBytes]
	}
	end := maxBytes - len(suffix)
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + suffix
}
