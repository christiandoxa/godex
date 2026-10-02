package kiro

import (
	"encoding/json"
	"fmt"
	"strings"
)

func runtimeTextContent(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case []any:
		return runtimeTextParts(typed)
	case map[string]any:
		text, _ := typed["text"].(string)
		return text, nil
	default:
		return "", nil
	}
}

func runtimeTextParts(parts []any) (string, error) {
	texts := make([]string, 0, len(parts))
	for _, raw := range parts {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := item["type"].(string)
		if !kiroTextContentKind(kind) {
			return "", fmt.Errorf("Kiro ACP only supports text content, got %q", kind)
		}
		if text, ok := item["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func kiroTextContentKind(kind string) bool {
	return kind == "" || kind == "text" || kind == "input_text" || kind == "output_text"
}

func runtimeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system", "developer":
		return "System"
	case "assistant":
		return "Assistant"
	case "tool", "function":
		return "Tool"
	default:
		return "User"
	}
}

func runtimeReasoningEffort(object map[string]any) string {
	if reasoning, ok := object["reasoning"].(map[string]any); ok {
		if effort, ok := reasoning["effort"].(string); ok {
			return strings.TrimSpace(effort)
		}
	}
	return runtimeString(object["reasoning_effort"], "")
}

func runtimeCompact(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	content, _ := json.Marshal(value)
	return string(content)
}
func runtimeString(value any, fallback string) string {
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	return fallback
}
func runtimeBool(value any) bool { current, _ := value.(bool); return current }
func runtimeNumber(value any) (float64, bool) {
	switch current := value.(type) {
	case json.Number:
		parsed, err := current.Float64()
		return parsed, err == nil
	case float64:
		return current, true
	}
	return 0, false
}
func runtimeEmptyArray(value any) bool {
	values, ok := value.([]any)
	return ok && len(values) == 0
}
func runtimeAutoChoice(value any) bool {
	if text, ok := value.(string); ok {
		return text == "auto"
	}
	return false
}
func runtimeTextFormat(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	kind, _ := object["type"].(string)
	return kind == "" || kind == "text"
}
