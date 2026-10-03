//go:build windows

package codex

import "strings"

func renderSessionPath(path, raw string) string {
	if width := sessionPathEscapeWidth(raw); width > 1 {
		return strings.ReplaceAll(path, "\\", strings.Repeat("\\", width))
	}
	return path
}
