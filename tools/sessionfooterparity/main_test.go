package main

import (
	"reflect"
	"testing"
)

func TestSplitShellCommand(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "posix hook", text: "'/tmp/godex' '__runtime-goal-session-notify' '/tmp/godex-session-hook/session.id'", want: []string{"/tmp/godex", "__runtime-goal-session-notify", "/tmp/godex-session-hook/session.id"}},
		{name: "spaces in path", text: "'/tmp/a path/codex' '__runtime-goal-session-notify' '/tmp/a path/id'", want: []string{"/tmp/a path/codex", "__runtime-goal-session-notify", "/tmp/a path/id"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := splitShellCommand(test.text); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("splitShellCommand() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestQuotedStringEndSkipsEscapedQuotes(t *testing.T) {
	value := `"a\"b" trailing`
	end := quotedStringEnd(value)
	if end != len(`"a\"b"`) {
		t.Fatalf("quotedStringEnd() = %d, want %d", end, len(`"a\"b"`))
	}
}
