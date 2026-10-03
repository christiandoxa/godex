package codex

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

type sessionLineMetadata struct {
	typeClass int
	resumeID  string
}

func sessionIDFromPath(path string) (string, bool) {
	name := filepath.Base(path)
	extension := filepath.Ext(name)
	stem := strings.TrimSuffix(name, extension)
	if fullSessionID(stem) {
		return stem, true
	}
	parts := strings.Split(stem, "-")
	for index := 0; index+5 <= len(parts); index++ {
		candidate := strings.Join(parts[index:index+5], "-")
		if fullSessionID(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func fullSessionID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index := 0; index < len(value); index++ {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if value[index] != '-' {
				return false
			}
			continue
		}
		if !sessionHexByte(value[index]) {
			return false
		}
	}
	return true
}

func sessionHexByte(value byte) bool {
	return value >= '0' && value <= '9' ||
		value >= 'a' && value <= 'f' ||
		value >= 'A' && value <= 'F'
}

func parseSessionLineMetadata(line string) (map[string]any, sessionLineMetadata, bool) {
	var value map[string]any
	if json.Unmarshal([]byte(line), &value) != nil || value == nil {
		return nil, sessionLineMetadata{}, false
	}
	payload, _ := value["payload"].(map[string]any)
	typeClass := 0
	if typeName, ok := value["type"].(string); ok {
		switch typeName {
		case "session_meta":
			typeClass = 1
		case "turn_context":
			typeClass = 2
		default:
			typeClass = 3
		}
	}
	resumeID := firstSessionString(
		sessionMapValue(payload, "id"),
		sessionMapValue(payload, "session_id"),
		value["id"],
		value["session_id"],
	)
	return value, sessionLineMetadata{typeClass: typeClass, resumeID: resumeID}, true
}

func sessionMapValue(object map[string]any, key string) any {
	if object == nil {
		return nil
	}
	return object[key]
}

func firstSessionString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok {
			if text = strings.TrimSpace(text); text != "" {
				return text
			}
		}
	}
	return ""
}

func sessionLineResumeID(line string) string {
	_, metadata, ok := parseSessionLineMetadata(line)
	if !ok {
		return ""
	}
	return metadata.resumeID
}

func sessionLineStartsResumeMetadata(line string) bool {
	_, metadata, ok := parseSessionLineMetadata(line)
	return ok && metadata.resumeID != "" && (metadata.typeClass == 0 || metadata.typeClass == 1)
}

func sessionLineResumeIDMatches(line, selector string) bool {
	id := sessionLineResumeID(line)
	return id != "" && sessionIDMatchesSelector(id, selector)
}

func sessionIDMatchesSelector(id, selector string) bool {
	return strings.EqualFold(id, selector) ||
		strings.HasPrefix(strings.ToLower(id), strings.ToLower(selector))
}

func sessionLineStartsCodexRolloutMetadata(line string) bool {
	value, _, ok := parseSessionLineMetadata(line)
	if !ok {
		return false
	}
	if _, ok := value["timestamp"].(string); !ok {
		return false
	}
	if typeName, _ := value["type"].(string); typeName != "session_meta" {
		return false
	}
	payload, ok := value["payload"].(map[string]any)
	if !ok {
		return false
	}
	for _, field := range []string{"id", "timestamp", "cwd", "originator", "cli_version"} {
		if _, ok := payload[field].(string); !ok {
			return false
		}
	}
	return true
}

func sessionLineValidJSON(line string) bool {
	return json.Valid([]byte(line))
}
