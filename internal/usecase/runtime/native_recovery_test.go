package runtime

import "testing"

func TestNativeResumeSessionIDOnlyAcceptsOwnedTUIResume(t *testing.T) {
	const id = "00000000-0000-7000-8000-000000000042"
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "resume", args: []string{"resume", id}, want: id},
		{name: "options", args: []string{"--model", "synthetic", "resume", id}, want: id},
		{name: "exec resume", args: []string{"exec", "resume", id}},
		{name: "remote resume", args: []string{"--remote", "unix:///tmp/foreign", "resume", id}},
		{name: "remote after resume", args: []string{"resume", id, "--remote", "unix:///tmp/foreign"}},
		{name: "invalid identity", args: []string{"resume", "not-a-session"}},
		{name: "prompt text", args: []string{"continue", "resume", id}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeResumeSessionID(test.args); got != test.want {
				t.Fatalf("native resume ID = %q, want %q", got, test.want)
			}
		})
	}
}
