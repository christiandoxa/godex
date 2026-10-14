//go:build windows

package codex

import (
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// Like the tagged Prodex 0.437.11 Windows terminal handoff, only an
// interactive foreground child may reset console terminal protocols.
// Preserve the inherited console mode as well; never emit escape
// sequences into redirected stdout, app-server, or machine-readable execs.
func captureOwnedChildTerminal043711(stdin io.Reader, stdout io.Writer) func() {
	input, inputOK := stdin.(*os.File)
	output, outputOK := stdout.(*os.File)
	if !inputOK || !outputOK || input == nil || output == nil ||
		!term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return func() {}
	}
	saved, err := term.GetState(int(input.Fd()))
	var once sync.Once
	return func() {
		once.Do(func() {
			_, _ = io.WriteString(stdout, ownedChildTerminalReset043711)
			if err == nil && saved != nil {
				_ = term.Restore(int(input.Fd()), saved)
			}
		})
	}
}
