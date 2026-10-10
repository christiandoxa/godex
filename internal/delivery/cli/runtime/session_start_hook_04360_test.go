package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func TestProdex04360SessionStartHookInjectsCanonicalTrustedConfig(t *testing.T) {
	dir := t.TempDir()
	marker := sessionStartMarker04360{directory: dir, path: filepath.Join(dir, "session.id")}
	for _, tc := range []struct {
		platform string
		wantKey  string
	}{
		{"linux", "/<session-flags>/config.toml:session_start:0:0"},
		{"windows", `C:\<session-flags>\config.toml:session_start:0:0`},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			command := sessionStartHookCommand04360([]string{"/usr/bin/godex", "__runtime-goal-session-notify", marker.path}, tc.platform)
			actual := sessionStartHookHash04360(command)
			if !strings.HasPrefix(actual, "sha256:") || len(actual) != 71 {
				t.Fatalf("invalid hook trust hash %q", actual)
			}
			args := marker.codexHookArgumentsForOS04360([]string{"exec", "--cyber-access-program", "standard", "task"}, "/usr/bin/godex", tc.platform)
			joined := strings.Join(args, "\n")
			literal, _ := json.Marshal(command)
			identity := "{\"event_name\":\"session_start\",\"hooks\":[{\"async\":false,\"command\":" +
				string(literal) + ",\"timeout\":5,\"type\":\"command\"}]}"
			digest := sha256.Sum256([]byte(identity))
			wantHash := "sha256:" + hex.EncodeToString(digest[:])
			if actual != wantHash {
				t.Fatalf("hook hash differs from the exact Prodex canonical identity: %q expected %q", actual, wantHash)
			}
			if !strings.Contains(joined, strconv.Quote(tc.wantKey)) {
				t.Fatalf("trusted hook identity key missing in %s", joined)
			}
			if !strings.Contains(joined, "hooks.SessionStart=") || !strings.Contains(joined, "hooks.state=") ||
				!strings.Contains(joined, actual) || !strings.Contains(joined, "--cyber-access-program\nstandard") {
				t.Fatalf("hook config or original Cyber options lost: %s", joined)
			}
			original := []string{"-c", "hooks.SessionStart=[]", "exec", "task"}
			custom := marker.codexHookArgumentsForOS04360(original, "/usr/bin/godex", tc.platform)
			if !reflect.DeepEqual(original, custom) {
				t.Fatalf("user-supplied SessionStart hook overridden: %#v", custom)
			}
			if len(args) < 6 || args[len(args)-1] != "task" {
				t.Fatalf("hook damaged original argv %#v", args)
			}
			if args[0] != "-c" || args[2] != "-c" {
				t.Fatalf("config overrides not prepended: %#v", args)
			}
		})
	}
}

func TestProdex04371GoalNotifyFallbackPreservesConfiguredNotify(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "session.id")
	args := addRuntimeGoalNotify04360(home, []string{"exec", "task"}, "/usr/bin/godex", marker)
	if len(args) < 2 || !strings.HasPrefix(args[1], "notify=[\"/usr/bin/godex\"") ||
		!strings.Contains(args[1], sessionStartNotifyCommand04360) {
		t.Fatalf("notify fallback was not injected: %#v", args)
	}
	configured := []string{"-c", `notify=["user-notifier"]`, "exec", "task"}
	if got := addRuntimeGoalNotify04360(home, configured, "/usr/bin/godex", marker); !reflect.DeepEqual(got, configured) {
		t.Fatalf("explicit notify override changed: %#v", got)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("notify = [\"user-notifier\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := addRuntimeGoalNotify04360(home, []string{"exec", "task"}, "/usr/bin/godex", marker); len(got) != 2 {
		t.Fatalf("configured notify was duplicated: %#v", got)
	}
}

func TestProdex04360PrivateSessionHookRejectsForgedAndOversizedNotifications(t *testing.T) {
	const session = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	monitor, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer monitor.Close()
	notify := func(raw string) (bool, error) {
		return HandleSessionStartNotify04360([]string{"__runtime-goal-session-notify", monitor.path}, strings.NewReader(raw))
	}
	for _, payload := range []string{
		`{"thread-id":"not-a-uuid"}`,
		`{"thread-id":"019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9","session_id":"00000000-0000-4000-8000-000000000002"}`,
		`{"session_id":""}`,
		strings.Repeat("x", 65537),
		"not JSON",
	} {
		handled, err := notify(payload)
		if !handled || err == nil {
			t.Fatalf("invalid payload accepted: handled %t error %v", handled, err)
		}
	}
	if id := monitor.ID(); id != "" {
		t.Fatalf("invalid notification created marker %q", id)
	}
	handled, err := notify(`{"thread-id":"` + session + `"}`)
	if !handled || err != nil {
		t.Fatalf("valid Codex marker failed: %t %v", handled, err)
	}
	if id := monitor.ID(); id != session {
		t.Fatalf("marker identity=%q expected %q", id, session)
	}
	if _, err := notify(`{"session_id":"` + session + `"}`); err == nil {
		t.Fatal("marker overwritten by repeated request")
	}
	external := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(external, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(monitor.directory, "symlink.id")); err == nil {
		if handled, err := HandleSessionStartNotify04360(
			[]string{"__runtime-goal-session-notify", filepath.Join(monitor.directory, "symlink.id")},
			bytes.NewReader([]byte(`{"session_id":"`+session+`"}`)),
		); !handled || err == nil {
			t.Fatal("symlink marker accepted")
		}
		data, _ := os.ReadFile(external)
		if string(data) != "preserve" {
			t.Fatal("target changed via symlink")
		}
	}
}

func TestProdex04360MarkerSelectsOnlyVerifiedNewSession(t *testing.T) {
	before := []sessionmodel.Report{{ID: "00000000-0000-4000-8000-000000000001", Path: "/old"}}
	a := sessionmodel.Report{ID: "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9", Path: "/sessions/a.jsonl", Source: "exec"}
	b := sessionmodel.Report{ID: "019c9e3d-45a0-7ad0-a6ee-b194ac2d44fa", Path: "/sessions/b.jsonl", Source: "exec"}
	for _, tc := range []struct {
		name, id string
		after    []sessionmodel.Report
		want     bool
	}{
		{"two new and authenticated marker", a.ID, []sessionmodel.Report{a, b}, true},
		{"two new without marker", "", []sessionmodel.Report{a, b}, false},
		{"marker points at historical session", before[0].ID, []sessionmodel.Report{a, b}, false},
		{"marker points at unknown session", "00000000-0000-4000-8000-000000000003", []sessionmodel.Report{a, b}, false},
		{"marker points to duplicate", a.ID, []sessionmodel.Report{a, a, b}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, ok := newSessionAfterMarker04360(before, tc.after, tc.id)
			if ok != tc.want || ok && report.ID != a.ID {
				t.Fatalf("candidate %+v selected %t expected %t", report, ok, tc.want)
			}
		})
	}
}
