//go:build windows

package codex

import "io"

// Windows native terminal restoration uses the child/ConPTY owner, not the
// Unix termios handoff. Never emit Unix escape sequences into Windows pipes.
func captureOwnedChildTerminal043711(io.Reader, io.Writer) func() {
	return func() {}
}
