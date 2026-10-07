package session

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

func applySessionMetadata(report *sessionentity.Session, line []byte) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return
	}

	payload := objectValue(value["payload"])
	metadata := objectValue(value["metadata"])
	payloadMetadata := objectValue(payload["metadata"])
	typeRaw, typePresent := sessionRawStringToken(value["type"])
	typeName := strings.TrimSpace(typeRaw)
	typeClass := 0
	if typePresent {
		switch typeRaw {
		case "session_meta":
			typeClass = 1
		case "turn_context":
			typeClass = 2
		default:
			typeClass = 3
		}
	}
	if report.Preview == "" {
		report.Preview = sessionPreview(typeName, payload)
	}

	if typeClass == 2 {
		if model, present := firstSessionString(payload["model"], value["model"]); present {
			report.LastModel = model
		}
		if effort, present := firstSessionString(
			payload["effort"], payload["reasoning_effort"],
			value["effort"], value["reasoning_effort"],
		); present {
			report.LastReasoningEffort = effort
		}
	}

	if typeClass == 0 || typeClass == 1 {
		if id, present := firstSessionString(payload["id"], payload["session_id"], value["id"], value["session_id"]); present {
			report.ID = id
		}
	}

	if threadName, present := firstSessionString(
		payload["thread_name"], payload["title"], payloadMetadata["thread_name"],
		value["thread_name"], value["title"], metadata["thread_name"],
	); present {
		report.ThreadName = threadName
	}
	if cwd, present := firstSessionString(
		payload["cwd"], payloadMetadata["cwd"], payload["workdir"],
		value["cwd"], metadata["cwd"], value["workdir"],
	); present {
		report.CWD = cwd
	}
	if provider, present := firstSessionString(
		payload["model_provider"], payloadMetadata["model_provider"],
		value["model_provider"], metadata["model_provider"],
	); present {
		report.ModelProvider = provider
	}
	if source := sessionSourceKind(payload["source"]); source != "" {
		report.Source = source
	} else if source := sessionSourceKind(value["source"]); source != "" {
		report.Source = source
	}
	if parent, present := sessionParentThreadID(value, payload); present {
		report.ParentThreadID = parent
	}

	if timestamp, present := firstSessionString(
		value["updated_at"], value["timestamp"], payload["updated_at"], payload["timestamp"],
	); present {
		report.UpdatedAt = timestamp
		if sortKey, ok := sessionTimestampSortKey(timestamp); ok {
			report.UpdatedUnix = sortKey
		}
	} else if epoch, present := firstSessionInt64(
		value["updated_at"], value["ts"], value["timestamp"],
		payload["updated_at"], payload["ts"], payload["timestamp"],
	); present {
		report.UpdatedUnix = epoch
		report.UpdatedAt = sessionFormatEpoch(epoch)
	}
}

func sessionParentThreadID(value, payload map[string]any) (string, bool) {
	for _, root := range []map[string]any{payload, value} {
		source := objectValue(root["source"])
		subagent := objectValue(source["subagent"])
		spawn := objectValue(subagent["thread_spawn"])
		if parent, present := sessionStringToken(spawn["parent_thread_id"]); present {
			return parent, true
		}
	}
	return firstSessionString(payload["parent_thread_id"], value["parent_thread_id"])
}

func sessionRawStringToken(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

func sessionStringToken(value any) (string, bool) {
	text, ok := sessionRawStringToken(value)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	return text, text != ""
}

func firstSessionString(values ...any) (string, bool) {
	for _, value := range values {
		if text, present := sessionStringToken(value); present {
			return text, true
		}
	}
	return "", false
}

func firstSessionInt64(values ...any) (int64, bool) {
	for _, value := range values {
		number, ok := value.(json.Number)
		if !ok {
			continue
		}
		parsed, err := strconv.ParseInt(string(number), 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func sessionTimestampSortKey(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.Unix(), true
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

func sessionFormatEpoch(epoch int64) string {
	return time.Unix(epoch, 0).In(time.Local).Format("2006-01-02 15:04:05 MST")
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
