//go:build !windows

package codex

func renderSessionPath(path, _ string) string {
	return path
}
