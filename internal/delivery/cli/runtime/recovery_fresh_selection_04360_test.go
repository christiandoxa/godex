package runtime

import (
	"testing"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

const newSessionID04360 = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
const otherSessionID04360 = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44fa"

func TestProdex04360FreshSessionSelectionOnlyAcceptsOneNewIdentity(t *testing.T) {
	before := []sessionmodel.Report{{ID: "00000000-0000-4000-8000-000000000001", Path: "/old"}}
	next := sessionmodel.Report{ID: newSessionID04360, Path: "/sessions/rollout-" + newSessionID04360 + ".jsonl", Source: "exec", ModelProvider: "openai"}
	for _, tc := range []struct {
		name  string
		after []sessionmodel.Report
		want  bool
	}{
		{"exactly one new session", append(append([]sessionmodel.Report{}, before...), next), true},
		{"not new", before, false},
		{"two simultaneous new sessions", append(append([]sessionmodel.Report{}, before...), next,
			sessionmodel.Report{ID: otherSessionID04360, Path: "/sessions/rollout-" + otherSessionID04360 + ".jsonl"}), false},
		{"foreign source", append(append([]sessionmodel.Report{}, before...),
			sessionmodel.Report{ID: newSessionID04360, Path: next.Path, Source: "mcp"}), false},
		{"missing path", append(append([]sessionmodel.Report{}, before...),
			sessionmodel.Report{ID: newSessionID04360}), false},
		{"invalid identity", append(append([]sessionmodel.Report{}, before...),
			sessionmodel.Report{ID: "not-a-uuid", Path: next.Path}), false},
		{"duplicate identity", append(append([]sessionmodel.Report{}, before...), next, next), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := newSessionAfter04360(before, tc.after)
			if ok != tc.want || ok && got.ID != newSessionID04360 {
				t.Fatalf("candidate %+v eligible=%t want %t", got, ok, tc.want)
			}
		})
	}
}
