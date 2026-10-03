//go:build windows

package codex

func decodeSessionPath(raw string) string {
	width := sessionPathEscapeWidth(raw)
	if width <= 1 {
		return raw
	}
	return decodeSessionPathEscaped(raw, width)
}
