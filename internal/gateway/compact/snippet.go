package compact

import (
	"fmt"
	"strings"
)

func localSnippet(raw any) (string, bool) {
	object, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	itemType := stringField(object, "type", "item")
	var snippet string
	switch itemType {
	case "message":
		snippet = messageSnippet(object)
	case "function_call", "custom_tool_call", "function_call_output", "custom_tool_call_output", "local_shell_call", "web_search_call":
		snippet = toolSnippet(itemType, object)
	case "reasoning":
		var found bool
		snippet, found = reasoningSnippet(object)
		if !found {
			return "", false
		}
	default:
		text := textFromContent(object)
		if strings.TrimSpace(text) == "" {
			return "", false
		}
		snippet = itemType + ": " + truncateUTF8(text, maxSnippetBytes)
	}
	return truncateUTF8(snippet, maxSnippetBytes), true
}

func messageSnippet(object map[string]any) string {
	role := stringField(object, "role", "unknown")
	text := textFromContent(object["content"])
	if text == "" {
		if candidate, ok := object["text"].(string); ok {
			text = candidate
		}
	}
	if strings.TrimSpace(text) == "" {
		return role + " message with no text content"
	}
	return role + " message: " + truncateUTF8(text, maxSnippetBytes)
}

func reasoningSnippet(object map[string]any) (string, bool) {
	summary := textFromContent(object["summary"])
	if strings.TrimSpace(summary) == "" {
		return "", false
	}
	return "reasoning summary: " + truncateUTF8(summary, maxSnippetBytes), true
}

func toolSnippet(itemType string, object map[string]any) string {
	callID := stringField(object, "call_id", "unknown")
	switch itemType {
	case "function_call":
		return fmt.Sprintf("tool call %s (%s): %s", stringField(object, "name", "function"), callID, truncateUTF8(textFromContent(object["arguments"]), maxSnippetBytes))
	case "custom_tool_call":
		return fmt.Sprintf("custom tool call %s (%s): %s", stringField(object, "name", "custom_tool"), callID, truncateUTF8(textFromContent(object["input"]), maxSnippetBytes))
	case "function_call_output", "custom_tool_call_output":
		return fmt.Sprintf("tool output %s: %s", callID, truncateUTF8(textFromContent(object["output"]), maxSnippetBytes))
	case "local_shell_call":
		return fmt.Sprintf("local shell call %s: %s", callID, truncateUTF8(textFromContent(object["action"]), maxSnippetBytes))
	case "web_search_call":
		return "web search: " + truncateUTF8(textFromContent(object["action"]), maxSnippetBytes)
	default:
		return ""
	}
}

func stringField(object map[string]any, key, fallback string) string {
	if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
