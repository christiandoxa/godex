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
		{[]string{"resume", "--", id}, 2},
		{[]string{"--", "resume", id}, -1},
		{[]string{"exec", "--", "resume", id}, -1},
		{[]string{"exec", "prompt", "resume", id}, -1},
		{[]string{"resume", "--last", "prompt"}, -1}, {[]string{"resume", "thread name"}, -1}, {[]string{"exec", id}, -1},
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
