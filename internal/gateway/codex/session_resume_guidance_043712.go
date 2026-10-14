package codex

import (
	"fmt"
	"io"
	"os"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"golang.org/x/term"
)

// printOwnedCompanionResumeGuidance043712 appends plain text after a final
// interactive child exit, including a non-zero child status. The three
// terminal checks keep diagnostics, pipes, and JSON protocols free of guidance.
func printOwnedCompanionResumeGuidance043712(process *CodexProcess, markerPath string, arguments []string) {
	if process == nil || !sessionTerminalTTY043712(process.terminal) {
		return
	}
	if flusher, ok := process.terminal.Stdout.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}
	id := savedSessionID043712(markerPath, arguments)
	_, _ = io.WriteString(process.terminal.Stderr, savedSessionGuidance043712(id))
}

func savedSessionID043712(markerPath string, arguments []string) string {
	if id := readSessionIdentityMarker(markerPath); id != "" {
		return id
	}
	return codexResumeSessionID(arguments)
}

func sessionTerminalTTY043712(terminal Terminal) bool {
	return terminalWriterTTY043712(terminal.Stdin) &&
		terminalWriterTTY043712(terminal.Stdout) &&
		terminalWriterTTY043712(terminal.Stderr)
}

func terminalWriterTTY043712(value any) bool {
	file, ok := value.(*os.File)
	return ok && file != nil && term.IsTerminal(int(file.Fd()))
}

func savedSessionGuidance043712(id string) string {
	if !sessionentity.ValidID(id) {
		id = ""
	}
	var resume string
	var identity string
	if id == "" {
		identity = "Session ID was not reported by this launch.\r\n"
		resume = "Resume: godex s resume\r\n"
	} else {
		identity = fmt.Sprintf("Session ID: %s\r\n", id)
		resume = "Resume: godex s " + id + "\r\n"
	}
	return "\r\nGodex saved session\r\n" +
		"This session's temporary app-server has stopped; its socket is closed.\r\n" +
		"The saved conversation and attachments remain available.\r\n" +
		identity + resume +
		"Do not reuse the temporary codex --remote socket shown above.\r\n" +
		"Paused goals stay paused until /goal resume.\r\n"
}
