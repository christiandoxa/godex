package codex

// Prodex 0.437.13 selects the normal screen without restoring an obsolete
// saved cursor (DEC 1047 rather than DEC 1049). It also leaves no mouse capture,
// bracketed paste, focus reporting, keyboard enhancement or hidden cursor
// behind on a child TUI exit. This protocol cleanup applies equally to
// Unix PTYs and interactive Windows consoles, never to redirected output.
const ownedChildTerminalReset043711 = "\x1b[?1047l" +
	"\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l" +
	"\x1b[?2004l\x1b[?1004l\x1b[<1u\x1b[?25h"
