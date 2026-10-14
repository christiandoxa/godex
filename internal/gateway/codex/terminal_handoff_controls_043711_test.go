package codex

import (
	"strings"
	"testing"
)

// Source-contract checks for the exact six kinds of terminal protocol
// cleanup required on Unix and Windows. The Linux PTY tests also verify
// that the handoff preserves an actual queued input stream.
func TestProdex043711CrossPlatformChildTerminalCleanupProtocols(t *testing.T) {
	for _, code := range []string{
		"\x1b[?1049l",
		"\x1b[?1000l",
		"\x1b[?1002l",
		"\x1b[?1003l",
		"\x1b[?1006l",
		"\x1b[?2004l",
		"\x1b[?1004l",
		"\x1b[<1u",
		"\x1b[?25h",
	} {
		if count := strings.Count(ownedChildTerminalReset043711, code); count != 1 {
			t.Fatalf("terminal cleanup %q appears %d times, want once", code, count)
		}
	}
}
