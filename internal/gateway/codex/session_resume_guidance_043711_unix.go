//go:build !windows

package codex

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Tagged Prodex 0.437.11 does not leave an ephemeral --remote socket as
// user-facing reconnect guidance once its session-owned app-server exits.
// Emit resume instructions only after a successful TTY child has exited and
// the private socket and companion have been reaped. Never mix terminal
// guidance into redirected stdout, JSON protocols or stderr logs.
func printOwnedCompanionResumeGuidance043711(stderr io.Writer) {
	file, ok := stderr.(*os.File)
	if !ok || file == nil || !term.IsTerminal(int(file.Fd())) {
		return
	}
	_, _ = io.WriteString(stderr, ownedCompanionResumeGuidance043711())
}

func ownedCompanionResumeGuidance043711() string {
	return "\nGodex saved session\n" +
		"This session's temporary app-server has stopped; its socket is closed.\n" +
		"The saved conversation and attachments remain available. Resume with: godex session resume <ID>\n" +
		"Do not reuse the temporary codex --remote socket shown above.\n" +
		"Paused goals stay paused until /goal resume.\n"
}
