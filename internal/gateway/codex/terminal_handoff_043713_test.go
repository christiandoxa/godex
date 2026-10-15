package codex

import (
	"strings"
	"testing"
)

// Prodex 0.437.13: DEC 1049's reset restores a saved cursor even after the
// child has already exited the alternate screen. DEC 1047 only selects the
// normal buffer, so repeated handoff cannot rewrite native Codex's footer.
func TestProdex043713TerminalHandoffNeverRestoresObsoleteCursor(t *testing.T) {
	id := "01900000-0000-7000-8000-000000000713"
	native := "Native usage line\r\nNative resume: " + id + "\r\n"
	output := native + ownedChildTerminalReset043711 + ownedChildTerminalReset043711 + savedSessionGuidance043712(id)
	if !strings.HasPrefix(output, native) || !strings.Contains(output[len(native):], "Session ID: "+id+"\r\n") {
		t.Fatalf("native terminal output or full session ID lost: %q", output)
	}
	if strings.Count(output, "\x1b[?1047l") != 2 {
		t.Fatalf("handoff must select normal buffer without cursor restore: %q", output)
	}
	for _, forbidden := range []string{"\x1b[?1049l", "\x1b[?1048l", "\x1b8", "\x1b[u", "\x1b[2J", "\x1b[3J"} {
		if strings.Contains(output[len(native):], forbidden) {
			t.Fatalf("handoff resets saved cursor or screen using %q: %q", forbidden, output)
		}
	}
}
