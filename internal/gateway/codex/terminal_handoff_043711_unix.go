//go:build !windows

package codex

import (
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// Tagged Prodex 0.437.11 leaves no raw/alternate terminal protocols behind
// after the interactive child exits. A noninteractive exec/app-server or
// redirected output never receives terminal escape sequences.
const ownedChildTerminalReset043711 = "\x1b[?1049l" +
	"\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l" +
	"\x1b[?2004l\x1b[?1004l\x1b[<1u\x1b[?25h"

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
