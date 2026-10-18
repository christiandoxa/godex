package jsonutil

import (
	"strings"
	"testing"
)

func TestSerdeBoundaryMatchesUnicodeAndRecursionLimits(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"high surrogate", `"\uD800"`, false},
		{"low surrogate", `"\uDC00"`, false},
		{"high then non-low", `"\uD800\u0041"`, false},
		{"surrogate pair", `"\uD83D\uDE00"`, true},
		{"escaped backslash", `"\\uD800"`, true},
		{"invalid UTF8", string([]byte{'"', 0xff, '"'}), false},
		{"invalid hex", `"\uXXXX"`, false},
		{"incomplete escape", `"\u12`, false},
		{"unterminated escape", `"\`, false},
		{"quoted brackets", `"[[{{"`, true},
		{"127 containers", strings.Repeat("[", 127) + "0" + strings.Repeat("]", 127), true},
		{"128 containers", strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if valid := CheckSerdeBoundary(tc.raw) == nil; valid != tc.valid {
				t.Fatalf("valid=%v want=%v", valid, tc.valid)
			}
		})
	}
}
