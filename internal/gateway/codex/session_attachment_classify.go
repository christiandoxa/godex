package codex

import (
	"path/filepath"
	"runtime"
	"strings"
)

const (
	sessionImageAttachmentDir = "image_attachments"
	sessionAttachmentDir      = "attachments"
	sessionImageTagPrefix     = "<image "
)

func sessionAttachmentPathSuffix(path string) (string, bool) {
	components := sessionPathComponents(path)
	index := -1
	for current, component := range components {
		if component == sessionAttachmentDir {
			index = current
			break
		}
	}
	if index < 0 || index+3 != len(components) {
		return "", false
	}
	id, name := components[index+1], components[index+2]
	if !sessionAttachmentComponent(id) || !sessionAttachmentComponent(name) ||
		!sessionPersistableAttachmentFileName(name) {
		return "", false
	}
	return filepath.Join(id, name), true
}

func sessionAttachmentComponent(value string) bool {
	return strings.TrimSpace(value) != "" && value != "." && value != ".." &&
		!strings.ContainsRune(value, filepath.Separator)
}

func sessionPathComponents(path string) []string {
	return sessionPathComponentsMode(path, runtime.GOOS == "windows")
}

func sessionPathComponentsMode(path string, windows bool) []string {
	var result []string
	start := 0
	for index := 0; index <= len(path); index++ {
		if index < len(path) && !sessionPathSeparatorMode(path[index], windows) {
			continue
		}
		if index > start {
			component := path[start:index]
			if component != "." {
				result = append(result, component)
			}
		}
		start = index + 1
	}
	return result
}

func sessionPathSeparatorMode(value byte, windows bool) bool {
	if windows {
		return value == '\\' || value == '/'
	}
	return value == '/'
}

func sessionAttachmentsAreStable(codexHome, contents string) bool {
	stableImages := filepath.Join(codexHome, sessionImageAttachmentDir)
	stableAttachments := filepath.Join(codexHome, sessionAttachmentDir)

	cursor := 0
	for {
		relative := strings.Index(contents[cursor:], sessionImageTagPrefix)
		if relative < 0 {
			break
		}
		tagStart := cursor + relative
		relativeEnd := strings.IndexByte(contents[tagStart:], '>')
		if relativeEnd < 0 {
			break
		}
		tagEnd := tagStart + relativeEnd
		tag := contents[tagStart:tagEnd]
		pathStart, pathEnd, ok := sessionImageTagPathRange(tag)
		if ok {
			path := decodeSessionPath(tag[pathStart:pathEnd])
			if filepath.IsAbs(path) && sessionClipboardFileName(filepath.Base(path)) &&
				!sessionPathStartsWith(path, stableImages) {
				return false
			}
		}
		cursor = tagEnd
	}

	cursor = 0
	for {
		start, end, ok := nextSessionClipboardPath(contents, cursor)
		if !ok {
			break
		}
		path := decodeSessionPath(contents[start:end])
		if filepath.IsAbs(path) && !sessionPathStartsWith(path, stableImages) {
			return false
		}
		cursor = end
	}

	cursor = 0
	for {
		start, end, ok := nextSessionAttachmentPath(contents, cursor)
		if !ok {
			break
		}
		path := decodeSessionPath(contents[start:end])
		_, persistable := sessionAttachmentPathSuffix(path)
		if filepath.IsAbs(path) && persistable && !sessionPathStartsWith(path, stableAttachments) {
			return false
		}
		cursor = end
	}
	return true
}

func sessionPersistedAttachmentPaths(contents string) []string {
	var paths []string
	add := func(path string) {
		for _, known := range paths {
			if known == path {
				return
			}
		}
		paths = append(paths, path)
	}

	cursor := 0
	for {
		relative := strings.Index(contents[cursor:], sessionImageTagPrefix)
		if relative < 0 {
			break
		}
		tagStart := cursor + relative
		relativeEnd := strings.IndexByte(contents[tagStart:], '>')
		if relativeEnd < 0 {
			break
		}
		tagEnd := tagStart + relativeEnd
		tag := contents[tagStart:tagEnd]
		start, end, ok := sessionImageTagPathRange(tag)
		if ok {
			path := decodeSessionPath(tag[start:end])
			if filepath.IsAbs(path) && sessionClipboardFileName(filepath.Base(path)) {
				add(path)
			}
		}
		cursor = tagEnd
	}

	cursor = 0
	for {
		start, end, ok := nextSessionClipboardPath(contents, cursor)
		if !ok {
			break
		}
		path := decodeSessionPath(contents[start:end])
		if filepath.IsAbs(path) {
			add(path)
		}
		cursor = end
	}

	cursor = 0
	for {
		start, end, ok := nextSessionAttachmentPath(contents, cursor)
		if !ok {
			break
		}
		path := decodeSessionPath(contents[start:end])
		if filepath.IsAbs(path) {
			if _, ok := sessionAttachmentPathSuffix(path); ok {
				add(path)
			}
		}
		cursor = end
	}
	return paths
}

func sessionPathStartsWith(path, root string) bool {
	pathParts := sessionPathComponents(path)
	rootParts := sessionPathComponents(root)
	if len(pathParts) < len(rootParts) {
		return false
	}
	for index := range rootParts {
		if pathParts[index] != rootParts[index] {
			return false
		}
	}
	return true
}
