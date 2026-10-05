package session

import (
	"encoding/json"
	"strings"
	"time"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

func applySessionMetadata(report *sessionentity.Session, line []byte) {
	var value map[string]any
	if json.Unmarshal(line, &value) != nil {
		return
	}
	payload := objectValue(value["payload"])
	metadata := objectValue(value["metadata"])
	payloadMetadata := objectValue(payload["metadata"])
	typeName := stringValue(value["type"])
	if report.Preview == "" {
		report.Preview = sessionPreview(typeName, payload)
	}

	if typeName != "" && typeName != "session_meta" && typeName != "turn_context" {
		return
	}
	if report.ID == "" && (typeName == "" || typeName == "session_meta") {
		report.ID = firstString(payload["id"], payload["session_id"], value["id"], value["session_id"])
	}
	if threadName := firstString(
		payload["thread_name"], payload["title"], payloadMetadata["thread_name"],
		value["thread_name"], value["title"], metadata["thread_name"],
	); threadName != "" {
		report.ThreadName = threadName
	}
	if cwd := firstString(
		payload["cwd"], payloadMetadata["cwd"], payload["workdir"],
		value["cwd"], metadata["cwd"], value["workdir"],
	); cwd != "" {
		report.CWD = cwd
	}
	if provider := firstString(
		payload["model_provider"], payloadMetadata["model_provider"],
		value["model_provider"], metadata["model_provider"],
	); provider != "" {
		report.ModelProvider = provider
	}
	if source := sessionSourceKind(payload["source"]); source != "" {
		report.Source = source
	} else if source := sessionSourceKind(value["source"]); source != "" {
		report.Source = source
	}
	if parent := sessionParentThreadID(value, payload); parent != "" {
		report.ParentThreadID = parent
	}
	if timestamp := firstString(value["updated_at"], value["timestamp"], payload["updated_at"], payload["timestamp"]); timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil && parsed.Unix() >= report.UpdatedUnix {
			report.UpdatedAt = timestamp
			report.UpdatedUnix = parsed.Unix()
		}
	}
}

func sessionParentThreadID(value, payload map[string]any) string {
	if parent := firstString(payload["parent_thread_id"], value["parent_thread_id"]); parent != "" {
		return parent
	}
	for _, root := range []map[string]any{payload, value} {
		source := objectValue(root["source"])
		subagent := objectValue(source["subagent"])
		spawn := objectValue(subagent["thread_spawn"])
		if parent := stringValue(spawn["parent_thread_id"]); parent != "" {
			return parent
		}
	}
	return ""
}

func objectValue(value any) map[string]any {
	object, _ := value.(map[string]any)
	if object == nil {
		return map[string]any{}
	}
	return object
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func sessionSourceKind(value any) string {
	switch typed := value.(type) {
	case string:
		source := strings.ToLower(strings.TrimSpace(typed))
		switch source {
		case "cli", "vscode", "exec", "mcp", "unknown":
			return source
		case "appserver", "app_server":
			return "mcp"
		case "":
			return ""
		default:
			return "unknown"
		}
	case map[string]any:
		for _, kind := range []string{"custom", "internal", "subagent"} {
			if _, ok := typed[kind]; ok {
				return kind
			}
		}
		return "unknown"
	default:
		return ""
	}
}

const codexUserMessagePrefix = "## My request for Codex:"

func sessionPreview(typeName string, payload map[string]any) string {
	var preview string
	switch typeName {
	case "event_msg":
		if stringValue(payload["type"]) == "user_message" {
			preview = stringValue(payload["message"])
		}
	case "response_item":
		if stringValue(payload["type"]) == "message" && strings.EqualFold(stringValue(payload["role"]), "user") {
			preview = responseItemUserText(payload["content"])
		}
	}
	return stripCodexUserMessagePrefix(preview)
}

func responseItemUserText(value any) string {
	items, ok := value.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		object := objectValue(item)
		if stringValue(object["type"]) != "input_text" {
			continue
		}
		if text := stringValue(object["text"]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func stripCodexUserMessagePrefix(text string) string {
	text = strings.TrimSpace(text)
	if index := strings.Index(text, codexUserMessagePrefix); index >= 0 {
		return strings.TrimSpace(text[index+len(codexUserMessagePrefix):])
	}
	return text
}
