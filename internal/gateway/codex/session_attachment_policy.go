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
	scanCursor := sessionScanCursor(contents, cursor)
	relative := strings.Index(contents[scanCursor:], sessionClipboardMarker)
	if relative < 0 {
		return 0, 0, false
	}
	markerStart := scanCursor + relative
	return expandSessionPath(contents, markerStart, len(sessionClipboardMarker), scanCursor)
}

func nextSessionAttachmentPath(contents string, cursor int) (int, int, bool) {
	if cursor < 0 || cursor > len(contents) {
		return 0, 0, false
	}
	scanCursor := sessionScanCursor(contents, cursor)
	markers := [...]string{"/attachments/", `/attachments\`, `\attachments/`, `\attachments\`}
	best := -1
	for _, marker := range markers {
		if relative := strings.Index(contents[scanCursor:], marker); relative >= 0 {
			candidate := scanCursor + relative
			if best < 0 || candidate < best {
				best = candidate
			}
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return expandSessionPath(contents, best, len("/attachments/"), scanCursor)
}

func sessionScanCursor(contents string, cursor int) int {
	if cursor < 0 || cursor >= len(contents) || contents[cursor] != 92 || cursor+1 >= len(contents) {
		return cursor
	}
	switch contents[cursor+1] {
	case 117:
		if cursor+6 <= len(contents) {
			return cursor + 6
		}
	case 34, 98, 102, 110, 114, 116:
		return cursor + 2
	}
	return cursor
}

func expandSessionPath(contents string, markerStart, markerLength, floor int) (int, int, bool) {
	if floor < 0 {
		floor = 0
	}
	if floor > markerStart {
		floor = markerStart
	}
	pathStart := markerStart
	for pathStart > floor && sessionPathByte(contents[pathStart-1]) {
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
