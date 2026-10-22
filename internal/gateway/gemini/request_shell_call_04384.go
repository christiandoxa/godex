package gemini

import (
	"strings"

	"github.com/christiandoxa/godex/internal/helper/jsonutil"
)

func projectGeminiChatShellCall04384(item map[string]any) bool {
	if !validGeminiChatShellCall04384(item) {
		return false
	}
	action, _ := item["action"].(map[string]any)
	command, present := item["command"]
	if !present {
		command = action["command"]
	}
	text, _ := command.(string)
	if values, ok := command.([]any); ok {
		parts := make([]string, 0, len(values))
		for _, value := range values {
			if part, ok := value.(string); ok {
				parts = append(parts, part)
			}
		}
		text = strings.Join(parts, " ")
	}
	encoded, err := jsonutil.MarshalSerdeValue04384(text)
	if err != nil {
		return false
	}
	parts := []string{`"command":` + string(encoded)}
	for _, key := range []string{"cwd", "timeout", "env"} {
		value, present := item[key]
		if !present {
			value, present = action[key]
		}
		if present {
			encoded, err := jsonutil.MarshalSerdeValue04384(value)
			if err != nil {
				return false
			}
			parts = append(parts, `"`+key+`":`+string(encoded))
		}
	}
	// The tagged embedded argument string keeps this field order instead of sorting its keys.
	item["arguments"] = "{" + strings.Join(parts, ",") + "}"
	item["type"], item["name"], item["call_id"] = "function_call", "shell_command", geminiChatWireCallIdentity04384(item)
	delete(item, "namespace")
	for _, field := range []string{"gemini_thought_signature", "thought_signature", "thoughtSignature", "provider_specific_fields", "extra_content"} {
		delete(item, field)
	}
	return true
}

func validGeminiChatShellCall04384(item map[string]any) bool {
	command := item["command"]
	if action, ok := item["action"].(map[string]any); ok {
		command = action["command"]
		if command != nil {
			parts, ok := command.([]any)
			if !ok || len(parts) == 0 {
				return false
			}
			for _, value := range parts {
				text, ok := value.(string)
				if !ok || strings.TrimSpace(text) == "" {
					return false
				}
			}
			return true
		}
	}
	text, _ := command.(string)
	return strings.TrimSpace(text) != ""
}
