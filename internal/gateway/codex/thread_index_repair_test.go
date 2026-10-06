package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCodexThreadIndexProtocolPaginatesActiveAndArchivedThreads(t *testing.T) {
	responses := strings.Join([]string{
		`{"id":1,"result":{}}`,
		`{"method":"server/status","params":{}}`,
		`{"id":2,"result":{"nextCursor":"active-next"}}`,
		`{"id":3,"result":{"nextCursor":null}}`,
		`{"id":4,"result":{"nextCursor":null}}`,
	}, "\n") + "\n"
	var requests strings.Builder
	if err := reconcileCodexThreadIndexProtocol(strings.NewReader(responses), &requests); err != nil {
		t.Fatal(err)
	}

	var messages []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(requests.String()), "\n") {
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	if len(messages) != 5 || messages[0]["method"] != "initialize" || messages[1]["method"] != "initialized" {
		t.Fatalf("requests = %#v", messages)
	}
	active := messages[2]["params"].(map[string]any)
	if active["archived"] != false || active["cursor"] != nil || active["limit"] != float64(100) || active["useStateDbOnly"] != false {
		t.Fatalf("active page = %#v", active)
	}
	if messages[3]["params"].(map[string]any)["cursor"] != "active-next" || messages[4]["params"].(map[string]any)["archived"] != true {
		t.Fatalf("pagination requests = %#v", messages[2:])
	}
}

func TestCodexThreadIndexProtocolMatchesProdexClientInfoAndIgnoresForeignStringID(t *testing.T) {
	responses := strings.Join([]string{
		`{"id":"foreign","result":{"ignored":true}}`,
		`{"id":1,"result":{}}`,
		`{"id":2,"result":{"nextCursor":null}}`,
		`{"id":3,"result":{"nextCursor":null}}`,
	}, "\n") + "\n"
	var requests strings.Builder
	if err := reconcileCodexThreadIndexProtocol(strings.NewReader(responses), &requests); err != nil {
		t.Fatal(err)
	}
	var initialize map[string]any
	if err := json.Unmarshal([]byte(strings.Split(strings.TrimSpace(requests.String()), "\n")[0]), &initialize); err != nil {
		t.Fatal(err)
	}
	client := initialize["params"].(map[string]any)["clientInfo"].(map[string]any)
	if client["name"] != "prodex-thread-index-reconciliation" || client["version"] != "0.435.6" {
		t.Fatalf("clientInfo = %#v", client)
	}
}

func TestCodexThreadIndexProtocolPreservesProdexServerErrorDetail(t *testing.T) {
	responses := `{"id":1,"error":{"message":"synthetic app-server detail"}}` + "\n"
	var requests strings.Builder
	err := reconcileCodexThreadIndexProtocol(strings.NewReader(responses), &requests)
	if err == nil || err.Error() != "Codex thread index reconciliation failed: synthetic app-server detail" {
		t.Fatalf("error = %v", err)
	}
}

func TestCodexThreadIndexProtocolRejectsRepeatedCursorAndServerErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		responses string
		want      string
	}{
		{
			name:      "repeated cursor",
			responses: `{"id":1,"result":{}}` + "\n" + `{"id":2,"result":{"nextCursor":"same"}}` + "\n" + `{"id":3,"result":{"nextCursor":"same"}}` + "\n",
			want:      "repeated",
		},
		{
			name:      "server error preserves app-server detail",
			responses: `{"id":1,"error":{"message":"synthetic detail"}}` + "\n",
			want:      "synthetic detail",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests strings.Builder
			err := reconcileCodexThreadIndexProtocol(strings.NewReader(test.responses), &requests)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCodexProcessRepairsThreadIndexUsingSelectedHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX shell")
	}
	root := t.TempDir()
	record := filepath.Join(root, "record")
	script := filepath.Join(root, "codex")
	activeHome := filepath.Join(root, "active-home")
	sharedHome := filepath.Join(root, "shared-home")
	if err := os.MkdirAll(activeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sharedHome, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sharedHome, "sessions"), filepath.Join(activeHome, "sessions")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	content := strings.Join([]string{
		"#!/bin/sh",
		"[ \"$1\" = app-server ] || exit 41",
		"printf '%s|%s|%s' \"$1\" \"$CODEX_HOME\" \"$CODEX_SQLITE_HOME\" > \"$GODEX_THREAD_INDEX_RECORD\"",
		"read line",
		"printf '%s\\n' '{\"id\":1,\"result\":{}}'",
		"read line",
		"read line",
		"printf '%s\\n' '{\"id\":2,\"result\":{\"nextCursor\":null}}'",
		"read line",
		"printf '%s\\n' '{\"id\":3,\"result\":{\"nextCursor\":null}}'",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_THREAD_INDEX_RECORD", record)
	t.Setenv("CODEX_HOME", filepath.Join(root, "ambient-home"))
	t.Setenv("CODEX_SQLITE_HOME", filepath.Join(root, "ambient-sqlite-home"))
	if err := NewCodexProcess(script, Terminal{}).ReconcileThreadIndex(t.Context(), activeHome, sharedHome); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "app-server|"+activeHome+"|"+sharedHome {
		t.Fatalf("child app-server environment = %q", got)
	}
}

func TestCodexThreadIndexEnvironmentPreservesSQLiteHomeWithoutSharedSessions(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "configured-sqlite-home")
	t.Setenv("CODEX_SQLITE_HOME", want)
	activeHome := filepath.Join(root, "active")
	sharedHome := filepath.Join(root, "shared")
	if err := os.MkdirAll(filepath.Join(activeHome, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sharedHome, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	var got string
	for _, entry := range codexThreadIndexEnvironment(activeHome, sharedHome) {
		if strings.HasPrefix(strings.ToUpper(entry), "CODEX_SQLITE_HOME=") {
			got = strings.TrimPrefix(entry, "CODEX_SQLITE_HOME=")
		}
	}
	if got != want {
		t.Fatalf("CODEX_SQLITE_HOME = %q, want %q", got, want)
	}
}

func TestCodexThreadIndexEnvironmentUsesSameHomeWithoutSessionsDirectory(t *testing.T) {
	home := filepath.Join(t.TempDir(), "codex-home")
	assertCodexSQLiteHome(t, codexThreadIndexEnvironment(home, filepath.Join(home, "nested", "..")), home)
}

func TestCodexThreadIndexEnvironmentResolvesMissingSessionsThroughSymlinkParent(t *testing.T) {
	root := t.TempDir()
	sharedHome := filepath.Join(root, "shared-home")
	activeAlias := filepath.Join(root, "active-alias")
	ambient := filepath.Join(root, "ambient-sqlite")
	if err := os.MkdirAll(sharedHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sharedHome, activeAlias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	t.Setenv("CODEX_SQLITE_HOME", ambient)
	assertCodexSQLiteHome(t, codexThreadIndexEnvironment(activeAlias, sharedHome), sharedHome)
}

func assertCodexSQLiteHome(t *testing.T, environment []string, want string) {
	t.Helper()
	var sqliteHome string
	for _, entry := range environment {
		if strings.HasPrefix(strings.ToUpper(entry), "CODEX_SQLITE_HOME=") {
			sqliteHome = strings.TrimPrefix(entry, "CODEX_SQLITE_HOME=")
		}
	}
	if sqliteHome != want {
		t.Fatalf("CODEX_SQLITE_HOME = %q, want %q", sqliteHome, want)
	}
}

func TestCodexProcessReconcileThreadIndexHonorsCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX shell")
	}
	root := t.TempDir()
	script := filepath.Join(root, "codex")
	content := "#!/bin/sh\nread line\nprintf '%s\\n' '{\"id\":1,\"result\":{}}'\nread line\nsleep 20\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := NewCodexProcess(script, Terminal{}).ReconcileThreadIndex(ctx, filepath.Join(root, "active"), "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("repair error = %v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatalf("repair cleanup exceeded bound: %s", time.Since(started))
	}
}
