package runtime

import (
	"reflect"
	"testing"
)

func TestNativeSessionArgumentForms(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	for _, test := range []struct {
		args  []string
		index int
	}{
		{[]string{id}, 1}, {[]string{"resume", id}, 1}, {[]string{"exec", "resume", id, "prompt"}, 2},
		{[]string{"fork", "--model", "synthetic", id}, 3}, {[]string{"delete", "--force", id}, 2},
		{[]string{"-c", "features.current_time_reminder=true", "resume", id}, 3},
		{[]string{"--model", "synthetic", "resume", id}, 3},
		{[]string{"-C", "/synthetic/project", "exec", "--json", "resume", id}, 5},
		{[]string{"--config=model=synthetic", "e", "fork", id}, 3},
		{[]string{"resume", "--profile", "abcd", id}, 3},
		{[]string{"resume", "--", id}, -1}, // Prodex 0.436.0 Mojo launch_first treats -- as a literal boundary.
		{[]string{"queue", "--thread", id, "--message", "message"}, 2},
		{[]string{"--model", "synthetic", "queue", "--message", "--thread=literal", "--thread=0000"}, 5},
		{[]string{"queue", "--message", id, "--thread", "thread name"}, -1},
		{[]string{"queue", "--", "--thread", id}, -1},
		{[]string{"--", "resume", id}, -1},
		{[]string{"exec", "--", "resume", id}, -1},
		{[]string{"exec", "prompt", "resume", id}, -1},
		{[]string{"resume", "--last", "prompt"}, 1}, {[]string{"resume", "thread name"}, 1},
		{[]string{"exec", "resume", "thread name", "prompt"}, 2}, {[]string{"fork", "--last", "prompt"}, 1},
		{[]string{"resume", "--remote", "ws://127.0.0.1:9000", "thread name"}, 3},
		{[]string{"fork", "--remote-auth-token-env", "CODEX_REMOTE_TOKEN", "--last"}, 3},
		{[]string{"fork", "thread name"}, -1}, {[]string{"exec", id}, -1},
	} {
		index, args := sessionArgument(test.args)
		if index != test.index {
			t.Fatalf("%v index=%d, want %d", test.args, index, test.index)
		}
		if len(test.args) > 1 && !reflect.DeepEqual(args, test.args) {
			t.Fatalf("arguments changed: %v", args)
		}
	}
}
