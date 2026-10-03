//go:build !windows

package codex

func decodeSessionPath(raw string) string {
	return raw
}
