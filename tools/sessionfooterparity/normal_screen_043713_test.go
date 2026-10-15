package main

import (
	"strings"
	"testing"
)

func TestProdex043713NormalScreenHandoffContract(t *testing.T) {
	prefix := "\x1b[?1049h\x1b[?1049lNative usage summary\r\nNative session summary\r\n"
	reset := "\x1b[?1047l\x1b[?1000l\x1b[?2004l\x1b[?25h"
	footer := "\r\nGodex saved session\r\nSession ID: " + fakeSessionID +
		"\r\nResume: godex s " + fakeSessionID + "\r\n"
	valid := prefix + reset + footer
	// A shorter handoff that omits buffer selection is not parity, even if
	// its prose footer still appears correctly.
	for _, tc := range []struct {
		name, output string
		valid        bool
	}{
		{"normal", valid, true},
		{"parent 1049 stale cursor", strings.Replace(valid, "\x1b[?1047l", "\x1b[?1049l", 1), false},
		{"parent 1048 saved cursor", strings.Replace(valid, "\x1b[?1047l", "\x1b[?1048l", 1), false},
		{"omitted safe buffer select", strings.Replace(valid, "\x1b[?1047l", "", 1), false},
		{"native output erased", strings.Replace(valid, "\x1b[?1047l", "\x1b[2J\x1b[?1047l", 1), false},
		{"truncated resume", strings.Replace(valid, fakeSessionID+"\r\n", "truncated\r\n", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkNativeNormalScreen(tc.output, 0, "godex")
			if (err == nil) != tc.valid {
				t.Fatalf("checkNativeNormalScreen accepted=%t want=%t err=%v", err == nil, tc.valid, err)
			}
		})
	}
}
