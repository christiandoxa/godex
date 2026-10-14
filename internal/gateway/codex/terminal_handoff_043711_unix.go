//go:build !windows

package codex

import (
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// captureOwnedChildTerminal043711 restores the *inherited* termios with
// TCSANOW semantics, not TCSAFLUSH, preserving queued input while resetting
// the child's mouse, paste, focus, keyboard, cursor and alternate-screen
// modes. Both input and output must be actual TTY file descriptors.
func captureOwnedChildTerminal043711(stdin io.Reader, stdout io.Writer) func() {
	noop := func() {}
	input, inputOK := stdin.(*os.File)
	output, outputOK := stdout.(*os.File)
	if !inputOK || !outputOK || input == nil || output == nil ||
		!term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return noop
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
