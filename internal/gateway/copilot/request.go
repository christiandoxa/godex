package copilot

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
)

const defaultRuntimeModel = "gpt-6-astra"
const copilotImageURLField = "image_url"

func canonicalizeCopilotRequest(body []byte) []byte {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return append([]byte(nil), body...)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return append([]byte(nil), body...)
	}
	if model, ok := object["model"].(string); ok {
		object["model"] = canonicalCopilotModel(model)
	}
	stripEncryptedContent(object, false)
	rewritten, err := json.Marshal(object)
	if err != nil {
		return append([]byte(nil), body...)
	}
	return rewritten
}

func canonicalCopilotModel(model string) string {
	trimmed := strings.TrimSpace(model)
	switch strings.ToLower(trimmed) {
	case "", "auto", "default", "codex", "astra":
		return defaultRuntimeModel
	case "sol":
		return "gpt-6.1-sol"
	case "luna":
		return "gpt-6-luna"
	case "sonnet", "claude":
		return "claude-sonnet-5-5"
	case "gemini":
		return "gemini-3.8-flash"
	default:
		return trimmed
	}
}

func stripEncryptedContent(value any, preserve bool) bool {
	switch current := value.(type) {
	case map[string]any:
		return stripEncryptedObject(current, preserve)
	case []any:
		return stripEncryptedArray(current, preserve)
	default:
		return false
	}
}

func stripEncryptedObject(object map[string]any, preserve bool) bool {
	objectPreserve := preserve || stringValue(object["type"]) == "compaction"
	changed := false
	for key, nested := range object {
		if key == "encrypted_content" && !objectPreserve {
			delete(object, key)
			changed = true
			continue
		}
		if stripEncryptedContent(nested, objectPreserve) {
			changed = true
		}
	}
	return changed
}

func stripEncryptedArray(values []any, preserve bool) bool {
	changed := false
	for _, nested := range values {
		if stripEncryptedContent(nested, preserve) {
			changed = true
		}
	}
	return changed
}

func copilotHasAgentInput(body []byte) bool {
	root, ok := decodeJSONObject(body)
	if !ok {
		return false
	}
	if chatMessagesHaveAgent(root["messages"]) {
		return true
	}
	return responsesInputHasAgent(root["input"])
}

func chatMessagesHaveAgent(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, ok := object["role"].(string)
		if !ok {
			continue
		}
		if strings.EqualFold(role, "assistant") || strings.EqualFold(role, "tool") {
			return true
		}
	}
	return false
}

func responsesInputHasAgent(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		roleValue, found := object["role"]
		if !found {
			continue
		}
		role, ok := roleValue.(string)
		if !ok {
			return true
		}
		trimmed := strings.TrimFunc(role, unicode.IsSpace)
		if trimmed == "" || strings.EqualFold(trimmed, "assistant") {
			return true
		}
	}
	return false
}

func copilotHasVisionInput(body []byte) bool {
	root, ok := decodeJSONObject(body)
	if !ok {
		return false
	}
	if responsesInputHasVision(root["input"]) {
		return true
	}
	return chatMessagesHaveVision(root["messages"])
}

func responsesInputHasVision(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if responsesImageObject(object) {
			return true
		}
		if stringValue(object["type"]) == "message" && responsesContentHasVision(object["content"]) {
			return true
		}
	}
	return false
}

func responsesContentHasVision(value any) bool {
	items, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		if object, ok := item.(map[string]any); ok && responsesImageObject(object) {
			return true
		}
	}
	return false
}

func responsesImageObject(object map[string]any) bool {
	if stringValue(object["type"]) != "input_image" {
		return false
	}
	return nonblankString(object[copilotImageURLField]) || nonblankString(object["file_id"])
}

func chatMessagesHaveVision(value any) bool {
	messages, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range messages {
		if chatMessageHasVision(item) {
			return true
		}
	}
	return false
}

func chatMessageHasVision(value any) bool {
	message, ok := value.(map[string]any)
	if !ok || stringValue(message["role"]) != "user" {
		return false
	}
	content, ok := message["content"].([]any)
	if !ok {
		return false
	}
	for _, part := range content {
		if chatImagePart(part) {
			return true
		}
	}
	return false
}

func chatImagePart(value any) bool {
	object, ok := value.(map[string]any)
	if !ok || stringValue(object["type"]) != copilotImageURLField {
		return false
	}
	image, ok := object[copilotImageURLField].(map[string]any)
	return ok && nonblankString(image["url"])
}

func decodeJSONObject(body []byte) (map[string]any, bool) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, false
	}
	return root, true
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func nonblankString(value any) bool {
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}
