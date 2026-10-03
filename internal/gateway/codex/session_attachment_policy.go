package codex

import "strings"

const sessionClipboardMarker = "codex-clipboard-"

func sessionImageTagPathRange(tag string) (int, int, bool) {
	if start := strings.Index(tag, `path=\"`); start >= 0 {
		pathStart := start + len(`path=\"`)
		if end := strings.Index(tag[pathStart:], `\"`); end >= 0 {
			return pathStart, pathStart + end, true
		}
		return 0, 0, false
	}
	start := strings.Index(tag, `path="`)
	if start < 0 {
		return 0, 0, false
	}
	pathStart := start + len(`path="`)
	end := strings.IndexByte(tag[pathStart:], '"')
	if end < 0 {
		return 0, 0, false
	}
	return pathStart, pathStart + end, true
}

func nextSessionClipboardPath(contents string, cursor int) (int, int, bool) {
	if cursor < 0 || cursor > len(contents) {
		return 0, 0, false
	}
	relative := strings.Index(contents[cursor:], sessionClipboardMarker)
	if relative < 0 {
		return 0, 0, false
	}
	markerStart := cursor + relative
	return expandSessionPath(contents, markerStart, len(sessionClipboardMarker))
}

func nextSessionAttachmentPath(contents string, cursor int) (int, int, bool) {
	if cursor < 0 || cursor > len(contents) {
		return 0, 0, false
	}
	markers := [...]string{"/attachments/", `/attachments\`, `\attachments/`, `\attachments\`}
	best := -1
	for _, marker := range markers {
		if relative := strings.Index(contents[cursor:], marker); relative >= 0 {
			candidate := cursor + relative
			if best < 0 || candidate < best {
				best = candidate
			}
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return expandSessionPath(contents, best, len("/attachments/"))
}

func expandSessionPath(contents string, markerStart, markerLength int) (int, int, bool) {
	pathStart := markerStart
	for pathStart > 0 && sessionPathByte(contents[pathStart-1]) {
		pathStart--
	}
	pathEnd := markerStart + markerLength
	for pathEnd < len(contents) && sessionPathContinues(contents, pathEnd) {
		pathEnd++
	}
	for pathEnd > markerStart && contents[pathEnd-1] == '.' {
		pathEnd--
	}
	present := pathStart < markerStart && pathEnd > markerStart+markerLength
	if !present {
		return 0, 0, false
	}
	return pathStart, pathEnd, true
}

func sessionPathByte(value byte) bool {
	switch value {
	case '"', '\'', '<', '>', '(', ')', '[', ']', '{', '}', ',', ';', ' ', '\t', '\r', '\n':
		return false
	default:
		return true
	}
}

func sessionPathContinues(contents string, index int) bool {
	if index < 0 || index >= len(contents) {
		return false
	}
	if contents[index] == '\\' {
		if index > 0 && contents[index-1] == '\\' {
			return true
		}
		end := index
		for end < len(contents) && contents[end] == '\\' {
			end++
		}
		jsonEscape := false
		if end < len(contents) {
			switch contents[end] {
			case '"', 'b', 'f', 'n', 'r', 't', 'u':
				jsonEscape = true
			}
		}
		if (end-index)%2 == 1 && jsonEscape {
			return false
		}
	}
	return sessionPathByte(contents[index])
}

func sessionClipboardFileName(name string) bool {
	return strings.HasPrefix(name, sessionClipboardMarker)
}

func sessionPersistableAttachmentFileName(name string) bool {
	return strings.HasPrefix(name, "pasted-text-") || strings.HasPrefix(name, "image-") || name == "goal-objective.md"
}
