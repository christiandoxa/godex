package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func rewriteSessionPersistedAttachmentPaths(codexHome, contents string) (string, error) {
	imageRewritten, err := rewriteSessionImagePaths(codexHome, contents)
	if err != nil {
		return "", err
	}
	clipboardRewritten, err := rewriteSessionInlineClipboardPaths(codexHome, imageRewritten)
	if err != nil {
		return "", err
	}
	return rewriteSessionInlineAttachmentPaths(codexHome, clipboardRewritten)
}

func rewriteSessionImagePaths(codexHome, contents string) (string, error) {
	var output strings.Builder
	output.Grow(len(contents))
	cursor := 0
	for {
		relativeTagStart := strings.Index(contents[cursor:], sessionImageTagPrefix)
		if relativeTagStart < 0 {
			break
		}
		tagStart := cursor + relativeTagStart
		relativeTagEnd := strings.IndexByte(contents[tagStart:], '>')
		if relativeTagEnd < 0 {
			break
		}
		tagEnd := tagStart + relativeTagEnd
		start, end, ok := sessionImageTagPathRange(contents[tagStart:tagEnd])
		if !ok {
			output.WriteString(contents[cursor:tagEnd])
			cursor = tagEnd
			continue
		}
		pathStart := tagStart + start
		pathEnd := tagStart + end
		replacement, err := stableSessionImagePath(codexHome, contents[pathStart:pathEnd])
		if err != nil {
			return "", err
		}
		output.WriteString(contents[cursor:pathStart])
		if replacement == "" {
			output.WriteString(contents[pathStart:pathEnd])
		} else {
			output.WriteString(replacement)
		}
		cursor = pathEnd
	}
	output.WriteString(contents[cursor:])
	return output.String(), nil
}

func rewriteSessionInlineClipboardPaths(codexHome, contents string) (string, error) {
	var output strings.Builder
	output.Grow(len(contents))
	cursor := 0
	for {
		start, end, ok := nextSessionClipboardPath(contents, cursor)
		if !ok {
			break
		}
		replacement, err := stableSessionImagePath(codexHome, contents[start:end])
		if err != nil {
			return "", err
		}
		output.WriteString(contents[cursor:start])
		if replacement == "" {
			output.WriteString(contents[start:end])
		} else {
			output.WriteString(replacement)
		}
		cursor = end
	}
	output.WriteString(contents[cursor:])
	return output.String(), nil
}

func rewriteSessionInlineAttachmentPaths(codexHome, contents string) (string, error) {
	var output strings.Builder
	output.Grow(len(contents))
	cursor := 0
	for {
		start, end, ok := nextSessionAttachmentPath(contents, cursor)
		if !ok {
			break
		}
		replacement, err := stableSessionAttachmentPath(codexHome, contents[start:end])
		if err != nil {
			return "", err
		}
		output.WriteString(contents[cursor:start])
		if replacement == "" {
			output.WriteString(contents[start:end])
		} else {
			output.WriteString(replacement)
		}
		cursor = end
	}
	output.WriteString(contents[cursor:])
	return output.String(), nil
}

func sessionAttachmentDestinationSafe(root, destination string) (bool, error) {
	if !sessionPathStartsWith(destination, root) {
		return false, nil
	}
	relative, err := filepath.Rel(root, filepath.Dir(destination))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, nil
	}
	// The shared attachment root may be a profile symlink; reject links below it.
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("inspect attachment destination directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return false, nil
		}
	}
	return true, nil
}

func stableSessionImagePath(codexHome, rawPath string) (string, error) {
	source := decodeSessionPath(rawPath)
	if !filepath.IsAbs(source) {
		return "", nil
	}
	name := filepath.Base(source)
	if !sessionClipboardFileName(name) {
		return "", nil
	}
	destination := filepath.Join(codexHome, sessionImageAttachmentDir, name)
	safe, err := sessionAttachmentDestinationSafe(
		filepath.Join(codexHome, sessionImageAttachmentDir), destination,
	)
	if err != nil {
		return "", err
	}
	if !safe {
		return "", nil
	}
	regular, err := sessionPathIsRegularFile(destination)
	if err != nil {
		return "", err
	}
	if !regular {
		if !sessionClipboardSourcePersistable(source) {
			return "", nil
		}
		sourceRegular, err := sessionPathIsRegularFile(source)
		if err != nil {
			return "", err
		}
		if !sourceRegular {
			return "", nil
		}
		if err := copySessionAttachmentFile(source, destination); err != nil {
			return "", err
		}
	}
	return renderSessionPath(destination, rawPath), nil
}

func stableSessionAttachmentPath(codexHome, rawPath string) (string, error) {
	source := decodeSessionPath(rawPath)
	if !filepath.IsAbs(source) {
		return "", nil
	}
	relative, ok := sessionAttachmentPathSuffix(source)
	if !ok {
		return "", nil
	}
	destination := filepath.Join(codexHome, sessionAttachmentDir, relative)
	safe, err := sessionAttachmentDestinationSafe(
		filepath.Join(codexHome, sessionAttachmentDir), destination,
	)
	if err != nil {
		return "", err
	}
	if !safe {
		return "", nil
	}
	regular, err := sessionPathIsRegularFile(destination)
	if err != nil {
		return "", err
	}
	if !regular {
		sourceRegular, err := sessionPathIsRegularFile(source)
		if err != nil {
			return "", err
		}
		if !sourceRegular {
			return "", nil
		}
		if err := copySessionAttachmentFile(source, destination); err != nil {
			return "", err
		}
	}
	return renderSessionPath(destination, rawPath), nil
}
