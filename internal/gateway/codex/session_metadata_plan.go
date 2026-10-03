package codex

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

const sessionRepairProdexVersion = "0.435.1"

func planSessionMetadataRepair(path, contents string) (string, bool, error) {
	selector, ok := sessionIDFromPath(path)
	if !ok {
		return contents, false, nil
	}
	lines := splitSessionRepairLines(contents)
	firstContent := -1
	for index, line := range lines {
		if strings.TrimSpace(line) != "" {
			firstContent = index
			break
		}
	}
	if firstContent < 0 {
		return contents, false, nil
	}

	firstMatching := sessionLineResumeIDMatches(lines[firstContent], selector) &&
		sessionLineStartsResumeMetadata(lines[firstContent])
	firstCodex := firstMatching && sessionLineStartsCodexRolloutMetadata(lines[firstContent])
	hasUnreadable := false
	for _, line := range lines {
		if strings.TrimSpace(line) != "" && !sessionLineValidJSON(line) {
			hasUnreadable = true
			break
		}
	}
	if firstCodex && !hasUnreadable {
		return contents, false, nil
	}

	metadataIndex := -1
	for index := firstContent + 1; index < len(lines); index++ {
		if sessionLineStartsCodexRolloutMetadata(lines[index]) &&
			sessionLineResumeIDMatches(lines[index], selector) {
			metadataIndex = index
			break
		}
	}

	metadataLine := ""
	switch {
	case firstCodex:
		metadataLine = lines[firstContent]
	case metadataIndex >= 0:
		metadataLine = lines[metadataIndex]
	default:
		metadataLine = syntheticSessionMetadataLine(path, selector, lines)
	}
	if metadataLine == "" {
		return contents, false, nil
	}

	var repaired strings.Builder
	repaired.WriteString(metadataLine)
	repaired.WriteByte('\n')
	for index, line := range lines {
		if firstMatching && index == firstContent ||
			index == metadataIndex ||
			strings.TrimSpace(line) == "" ||
			!sessionLineValidJSON(line) ||
			sessionLineStartsResumeMetadata(line) && sessionLineResumeIDMatches(line, selector) {
			continue
		}
		repaired.WriteString(line)
		repaired.WriteByte('\n')
	}
	return repaired.String(), true, nil
}

func splitSessionRepairLines(contents string) []string {
	raw := strings.Split(contents, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	for index := range raw {
		raw[index] = strings.TrimSuffix(raw[index], "\r")
	}
	return raw
}

func syntheticSessionMetadataLine(path, selector string, lines []string) string {
	sessionID := ""
	for _, line := range lines {
		id := sessionLineResumeID(line)
		if id != "" && sessionIDMatchesSelector(id, selector) {
			sessionID = id
			break
		}
	}
	if sessionID == "" {
		if id, ok := sessionIDFromPath(path); ok && sessionIDMatchesSelector(id, selector) {
			sessionID = id
		}
	}
	if sessionID == "" && fullSessionID(selector) {
		sessionID = selector
	}
	if sessionID == "" {
		return ""
	}

	timestamp := firstSessionLineString(lines,
		[]string{"timestamp"},
		[]string{"payload", "timestamp"},
	)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	}
	cwd := firstSessionLineString(lines, []string{"payload", "cwd"}, []string{"cwd"})
	if cwd == "" {
		cwd, _ = os.Getwd()
		if cwd == "" {
			cwd = "."
		}
	}
	modelProvider := firstSessionLineString(lines,
		[]string{"payload", "model_provider"},
		[]string{"model_provider"},
	)
	payload := map[string]any{
		"session_id":  sessionID,
		"id":          sessionID,
		"timestamp":   timestamp,
		"cwd":         cwd,
		"originator":  "prodex-repair",
		"cli_version": sessionRepairProdexVersion,
		"source":      "cli",
	}
	if modelProvider != "" {
		payload["model_provider"] = modelProvider
	}
	value := map[string]any{
		"timestamp": timestamp,
		"type":      "session_meta",
		"payload":   payload,
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func firstSessionLineString(lines []string, paths ...[]string) string {
	for _, line := range lines {
		var value map[string]any
		if json.Unmarshal([]byte(line), &value) != nil {
			continue
		}
		for _, path := range paths {
			var current any = value
			ok := true
			for _, segment := range path {
				object, objectOK := current.(map[string]any)
				if !objectOK {
					ok = false
					break
				}
				current, ok = object[segment]
				if !ok {
					break
				}
			}
			if ok {
				if text, textOK := current.(string); textOK {
					if text = strings.TrimSpace(text); text != "" {
						return text
					}
				}
			}
		}
	}
	return ""
}
