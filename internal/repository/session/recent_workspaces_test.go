package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

func TestProdex04382RecentWorkspacesReadInitialMetadataAndFileMtime(t *testing.T) {
	home := t.TempDir()
	metadata := func(cwd string) string {
		value, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]any{"id": "01900000-0000-7000-8000-000000000001", "cwd": cwd}, "timestamp": "2099-01-01T00:00:00Z"})
		return string(value)
	}
	a, b, c := filepath.Join(home, "a"), filepath.Join(home, "b"), filepath.Join(home, "c")
	for _, item := range []struct {
		dir, name, body string
		modified        int64
	}{
		{"sessions", "rollout-a.jsonl", "\n" + metadata(a) + "\n" + `{"type":"turn_context","payload":{"cwd":"/ignored-later-context"}}`, 100},
		{"sessions", "rollout-b.jsonl", metadata(b), 200},
		{"archived_sessions", "rollout-c.json", metadata(c), 200},
		{"sessions", "rollout-relative.jsonl", metadata("relative/workspace"), 300},
		{"sessions", "imported-session.jsonl", metadata(filepath.Join(home, "imported")), 400},
		{"sessions", "rollout-preepoch.jsonl", metadata(a), -100},
		{"sessions", "rollout-invalid.jsonl", "not metadata\n" + metadata(filepath.Join(home, "invalid")), 400},
	} {
		path := filepath.Join(home, item.dir, item.name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(item.body), 0600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(item.modified, 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := NewReader().Workspaces(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		if candidate.ModifiedUnix < 0 {
			t.Fatal("pre-epoch filesystem time must use the tagged zero fallback")
		}
	}
	got := sessionentity.RecentWorkspaces(candidates, 4096)
	if want := []string{filepath.Join(home, "imported"), b, c, a}; !reflect.DeepEqual(got, want) {
		t.Fatalf("workspaces=%q want=%q", got, want)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewReader().Workspaces(ctx, home); err == nil {
		t.Fatal("cancelled workspace scan succeeded")
	}
}

func TestProdex04382WorkspaceAliasesPreserveRawAndCanonicalPaths(t *testing.T) {
	root := t.TempDir()
	path := root + string(filepath.Separator) + "child" + string(filepath.Separator) + ".."
	if err := os.Mkdir(filepath.Join(root, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	aliases, err := NewReader().WorkspaceAliases(context.Background(), path)
	if err != nil || len(aliases) < 2 || aliases[0] != path || aliases[1] != root {
		t.Fatalf("aliases=%q err=%v", aliases, err)
	}
}

func TestProdex04382WorkspaceLegacyJSONAndInvalidMetadata(t *testing.T) {
	home := t.TempDir()
	initial, later := filepath.Join(home, "initial"), filepath.Join(home, "later")
	meta, _ := json.Marshal(map[string]any{"type": "session_meta", "payload": map[string]any{"id": "synthetic-session", "cwd": initial}})
	turn, _ := json.Marshal(map[string]any{"type": "turn_context", "payload": map[string]any{"cwd": later}})
	pretty, _ := json.MarshalIndent(map[string]any{"id": "synthetic-session", "cwd": initial}, "", "  ")
	missingID, _ := json.Marshal(map[string]any{"cwd": initial})
	for _, tc := range []struct{ name, body, want string }{
		{"rollout-pretty.json", string(pretty), initial},
		{"rollout-lines.json", string(meta) + "\n" + string(turn), later},
		{"rollout-lines.jsonl", string(meta) + "\n" + string(turn), initial},
		{"rollout-invalid.json", string(missingID), ""},
		{"rollout-invalid.jsonl", string(turn) + "\n" + string(meta), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(home, tc.name)
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := initialWorkspace(context.Background(), path)
			if err != nil || got != tc.want {
				t.Fatalf("workspace=%q want=%q err=%v", got, tc.want, err)
			}
		})
	}
}

func TestProdex04382WorkspaceMetadataRejectsInvalidSerdeJSON(t *testing.T) {
	for _, tail := range []string{
		`"cwd":"/synthetic/\uD800"`,
		`"cwd":"/synthetic/\uDC00"`,
		`"cwd":"/synthetic/valid","value":1e309`,
		`"cwd":"/synthetic/valid","value":` + strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128),
	} {
		content := []byte(`{"id":"synthetic-session",` + tail + `}`)
		if _, valid := workspaceMetadata(content); valid {
			t.Fatalf("malformed session JSON accepted: %s", content)
		}
	}
	for _, cwd := range []string{`/synthetic/\uD83D\uDE00`, `/synthetic/\\uD800`} {
		if report, valid := workspaceMetadata([]byte(`{"id":"synthetic-session","cwd":"` + cwd + `"}`)); !valid || report.CWD == "" {
			t.Fatal("valid surrogate/escaped path rejected")
		}
	}
}
