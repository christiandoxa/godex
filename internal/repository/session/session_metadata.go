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
