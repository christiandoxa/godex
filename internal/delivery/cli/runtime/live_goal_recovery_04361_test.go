package runtime

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
	sessionrepo "github.com/christiandoxa/godex/internal/repository/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"

	_ "modernc.org/sqlite"
)

type liveGoalProcess04361 struct {
	dbPath   string
	id       string
	calls    int
	homes    []string
	stale    bool
	started  chan struct{}
	trigger  chan struct{}
	released *atomic.Int32
}

type liveGoalQuota04361 struct{ backupReady *atomic.Bool }

func (quota liveGoalQuota04361) Ready(_ context.Context, account accountentity.Account) (bool, error) {
	return account.ID == "a" || account.ID == "b" && quota.backupReady.Load(), nil
}

func (process *liveGoalProcess04361) Run(ctx context.Context, home string, args []string) error {
	process.calls++
	process.homes = append(process.homes, home)
	if process.calls > 1 {
		if process.released == nil || process.released.Load() != 1 ||
			!containsArg04361(args, process.id) || !containsArg04361(args, "/goal resume") ||
			containsArg04361(args, "original task") {
			return errors.New("live recovery did not resume the goal")
		}
		return nil
	}
	marker := markerPathFromArgs04361(args)
	if marker == "" {
		return errors.New("live recovery hook marker missing: " + strings.Join(args, " | "))
	}
	if err := os.WriteFile(marker, []byte(process.id+"\n"), 0o600); err != nil {
		return err
	}
	if process.stale {
		return errors.New("stale goal child exit")
	}
	if process.started != nil {
		close(process.started)
	}
	<-process.trigger
	<-ctx.Done()
	return ctx.Err()
}

func containsArg04361(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestProdex04361LiveGoalLaunchScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "interactive prompt", args: []string{"continue the task"}, want: true},
		{name: "exec", args: []string{"exec", "continue the task"}, want: true},
		{name: "resume", args: []string{"resume", "session"}, want: false},
		{name: "review", args: []string{"exec", "review", "--uncommitted"}, want: false},
		{name: "app server", args: []string{"app-server"}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := liveGoalLaunch04361(tc.args); got != tc.want {
				t.Fatalf("live goal launch=%t want=%t for %#v", got, tc.want, tc.args)
			}
		})
	}
}

func markerPathFromArgs04361(args []string) string {
	for _, arg := range args {
		start := strings.Index(arg, "godex-session-hook-")
		if start < 0 {
			continue
		}
		end := strings.Index(arg[start:], "session.id")
		if end < 0 {
			continue
		}
		path := arg[start : start+end+len("session.id")]
		path = strings.ReplaceAll(path, "'\"'\"'", "'")
		path = strings.Trim(path, "'\"")
		if !filepath.IsAbs(path) {
			path = filepath.Join(os.TempDir(), path)
		}
		return path
	}
	return ""
}

func liveGoalFixture04361(t *testing.T, status string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	primary := filepath.Join(root, "primary")
	backup := filepath.Join(root, "backup")
	for _, path := range []string{shared, primary, backup} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	dbPath := fixtureGoalDB04360(t, shared, id, status)
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE thread_goals SET updated_at_ms=1 WHERE thread_id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	return root, shared, id
}

func liveGoalProfiles04361(root string) (*runPolicyAccounts, *verifiedRecoveryProfiles04360) {
	accounts := &runPolicyAccounts{
		values: []accountentity.Account{
			{ID: "a", Name: "primary", Enabled: true},
			{ID: "b", Name: "backup", Enabled: true},
		},
		homes: map[string]string{
			"a": filepath.Join(root, "primary"),
			"b": filepath.Join(root, "backup"),
		},
	}
	profiles := &verifiedRecoveryProfiles04360{
		fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{
			target: profilemodel.LaunchTarget{
				Name: "primary", CodexHome: filepath.Join(root, "primary"),
				AccountID: "a", Provider: "openai", Auth: "chatgpt",
			},
			active: true,
		},
		eligible: true,
	}
	return accounts, profiles
}

func TestProdex04361LiveGoalLimitCancelsChildReleasesAffinityAndResumesGoal(t *testing.T) {
	root, shared, id := liveGoalFixture04361(t, "active")
	accounts, profiles := liveGoalProfiles04361(root)
	process := &liveGoalProcess04361{
		dbPath: filepath.Join(shared, "goals_1.sqlite"), id: id,
		started: make(chan struct{}), trigger: make(chan struct{}),
	}
	var released atomic.Int32
	process.released = &released
	goalStateStore := runtimerepo.NewGoalStateStore(shared)
	defer goalStateStore.Close()
	runner := runtimeusecase.NewRunner(accounts, process, nil, goalStateStore)
	runner.SetSharedCodexHome(shared)
	var backupReady atomic.Bool
	runner.SetQuotaPreflight(liveGoalQuota04361{backupReady: &backupReady})
	sessions := sessionusecase.NewCatalog(accounts, sessionrepo.NewReader(), nil)
	sessions.SetSharedCodexHome(shared)
	sessions.SetBindingForget(func(context.Context, string) error {
		released.Add(1)
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- RunProfiles(t.Context(), runner, sessions, profiles, []string{"exec", "original task"}, io.Discard)
	}()
	<-process.started
	database, err := sql.Open("sqlite", process.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"UPDATE thread_goals SET status='usage_limited',updated_at_ms=? WHERE thread_id=?",
		time.Now().UnixMilli()+1000, id,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	backupReady.Store(true)
	close(process.trigger)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if process.calls != 2 || released.Load() != 1 {
		t.Fatalf("live recovery calls=%d affinity releases=%d", process.calls, released.Load())
	}
	if process.homes[1] != filepath.Join(root, "backup") {
		t.Fatalf("live recovery home=%q", process.homes[1])
	}
	if released.Load() != 1 {
		t.Fatalf("session affinity releases=%d", released.Load())
	}
}

func TestProdex04361LiveGoalRecoveryRejectsStaleStateAndNoAutoRotate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		noRotation bool
	}{
		{name: "stale historical limit", status: "usage_limited"},
		{name: "explicit no auto rotate", status: "active", noRotation: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, shared, id := liveGoalFixture04361(t, tc.status)
			accounts, profiles := liveGoalProfiles04361(root)
			process := &liveGoalProcess04361{
				dbPath: filepath.Join(shared, "goals_1.sqlite"), id: id, stale: true,
			}
			goalStateStore := runtimerepo.NewGoalStateStore(shared)
			defer goalStateStore.Close()
			runner := runtimeusecase.NewRunner(accounts, process, nil, goalStateStore)
			runner.SetSharedCodexHome(shared)
			sessions := sessionusecase.NewCatalog(accounts, sessionrepo.NewReader(), nil)
			released := 0
			sessions.SetBindingForget(func(context.Context, string) error {
				released++
				return nil
			})
			args := []string(nil)
			if tc.noRotation {
				args = []string{"--no-auto-rotate"}
			}
			if err := RunProfiles(t.Context(), runner, sessions, profiles, args, io.Discard); err == nil {
				t.Fatal("failed child unexpectedly succeeded")
			}
			if process.calls != 1 || released != 0 {
				t.Fatalf("negative recovery calls=%d affinity releases=%d", process.calls, released)
			}
		})
	}
}
