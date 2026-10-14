package codex

// Tagged Prodex 0.437.11 leaves no alternate screen, mouse capture,
// bracketed paste, focus reporting, keyboard enhancement or hidden cursor
// behind on a child TUI exit. This protocol cleanup applies equally to
// Unix PTYs and interactive Windows consoles, never to redirected output.
const ownedChildTerminalReset043711 = "\x1b[?1049l" +
	"\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l" +
	"\x1b[?2004l\x1b[?1004l\x1b[<1u\x1b[?25h"
