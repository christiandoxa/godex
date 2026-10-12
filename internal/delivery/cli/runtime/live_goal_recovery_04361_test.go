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
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
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

type liveGoalMultiProfiles04374 struct {
	*fakeLocalLaunchProfiles
	targets map[string]profilemodel.LaunchTarget
	homes   []sessionmodel.ProfileHome
}

func (profiles *liveGoalMultiProfiles04374) ResolveLaunch(_ context.Context, name string) (profilemodel.LaunchTarget, error) {
	target, ok := profiles.targets[name]
	if !ok {
		return profilemodel.LaunchTarget{}, errors.New("profile unavailable")
	}
	return target, nil
}

func (profiles *liveGoalMultiProfiles04374) SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error) {
	return append([]sessionmodel.ProfileHome(nil), profiles.homes...), nil
}

type liveGoalMultiQuota04374 struct{ ready *atomic.Bool }

func (quota liveGoalMultiQuota04374) Ready(_ context.Context, account accountentity.Account) (bool, error) {
	return account.ID == "a" || quota.ready.Load(), nil
}

type liveGoalMultiProcess04374 struct {
	marker  string
	rollout string
	session string
	calls   []string
	started chan struct{}
	trigger chan struct{}
}

func (process *liveGoalMultiProcess04374) Run(ctx context.Context, home string, args []string) error {
	process.calls = append(process.calls, home)
	switch len(process.calls) {
	case 1:
		process.marker = markerPathFromArgs04361(args)
		if process.marker == "" {
			return errors.New("live recovery hook marker missing")
		}
		if err := os.WriteFile(process.marker, []byte(process.session+"\n"), 0o600); err != nil {
			return err
		}
		close(process.started)
		<-process.trigger
		<-ctx.Done()
		return ctx.Err()
	case 2:
		if home == "" || containsArg04361(args, "original task") || !containsArg04361(args, "/goal resume") {
			return errors.New("live recovery did not resume the same goal")
		}
		file, err := os.OpenFile(process.rollout, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = file.WriteString(
			`{"type":"response_item","payload":{"type":"message","role":"user"}}` + "\n" +
				`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n",
		)
		if closeErr := file.Close(); err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return errors.New("synthetic second-profile overload")
	default:
		if home == "" || containsArg04361(args, "original task") || !containsArg04361(args, "/goal resume") {
			return errors.New("third recovery did not resume the same goal")
		}
		return nil
	}
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

func TestProdex04374LiveGoalRecoveryUsesPlannerAcrossGenerations(t *testing.T) {
	root, shared, id := liveGoalFixture04361(t, "active")
	third := filepath.Join(root, "third")
	if err := os.MkdirAll(third, 0o700); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(shared, "sessions", "2026", "10", "11", "rollout-2026-10-11T10-00-00-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(rollout), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rollout, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	accounts := &runPolicyAccounts{values: []accountentity.Account{
		{ID: "a", Name: "primary", Enabled: true},
		{ID: "b", Name: "backup", Enabled: true},
		{ID: "c", Name: "third", Enabled: true},
	}, homes: map[string]string{
		"a": filepath.Join(root, "primary"), "b": filepath.Join(root, "backup"), "c": third,
	}}
	primary := profilemodel.LaunchTarget{
		Name: "primary", CodexHome: filepath.Join(root, "primary"),
		AccountID: "a", Provider: "openai", Auth: "chatgpt",
	}
	profiles := &liveGoalMultiProfiles04374{
		fakeLocalLaunchProfiles: &fakeLocalLaunchProfiles{target: primary, active: true},
		targets: map[string]profilemodel.LaunchTarget{
			"primary": primary,
			"backup":  {Name: "backup", CodexHome: filepath.Join(root, "backup"), AccountID: "b", Provider: "openai", Auth: "chatgpt"},
			"third":   {Name: "third", CodexHome: third, AccountID: "c", Provider: "openai", Auth: "chatgpt"},
		},
		homes: []sessionmodel.ProfileHome{
			{Name: "primary", AccountID: "a", Provider: "openai", Enabled: true},
			{Name: "backup", AccountID: "b", Provider: "openai", Enabled: true},
			{Name: "third", AccountID: "c", Provider: "openai", Enabled: true},
		},
	}
	process := &liveGoalMultiProcess04374{
		rollout: rollout, session: id, started: make(chan struct{}), trigger: make(chan struct{}),
	}
	goalStore := runtimerepo.NewGoalStateStore(shared)
	defer goalStore.Close()
	runner := runtimeusecase.NewRunner(accounts, process, nil, goalStore)
	runner.SetSharedCodexHome(shared)
	var backupsReady atomic.Bool
	runner.SetQuotaPreflight(liveGoalMultiQuota04374{ready: &backupsReady})
	sessions := sessionusecase.NewCatalog(accounts, sessionrepo.NewReader(), nil)
	sessions.SetSharedCodexHome(shared)
	var released atomic.Int32
	sessions.SetBindingForget(func(context.Context, string) error {
		released.Add(1)
		return nil
	})

	done := make(chan error, 1)
	go func() {
		done <- RunProfiles(t.Context(), runner, sessions, profiles, []string{"exec", "original task"}, io.Discard)
	}()
	<-process.started
	database, err := sql.Open("sqlite", filepath.Join(shared, "goals_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		"UPDATE thread_goals SET status='usage_limited',updated_at_ms=? WHERE thread_id=?",
		time.Now().UnixMilli()+1000, id,
	); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	backupsReady.Store(true)
	close(process.trigger)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(process.calls) != 3 || released.Load() != 2 {
		t.Fatalf("live multi-generation calls=%d homes=%v releases=%d", len(process.calls), process.calls, released.Load())
	}
	if process.calls[1] != filepath.Join(root, "backup") || process.calls[2] != third {
		t.Fatalf("live planner selected homes=%v", process.calls)
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

func TestProdex04371LiveGoalRecoveryUsesNewRolloutEvidence(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	initial := `{"type":"session_meta","payload":{"id":"` + id + `"}}
{"type":"response_item","payload":{"role":"user","content":"continue"}}
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" {
		t.Fatalf("initial live goal state = %#v, ok=%t", state, ok)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"type":"response_item","payload":{"role":"user","content":"next turn"}}
{"type":"task_complete","error":{"codex_error_info":"usage_limit_exceeded"}}
`)
	if closeErr := file.Close(); err != nil || closeErr != nil {
		t.Fatalf("append rollout evidence: write=%v close=%v", err, closeErr)
	}
	state, ok := monitor.readState(t.Context())
	if !ok || state.Status != "usage_limited" || state.SessionID != id {
		t.Fatalf("rollout evidence did not trigger recovery: %#v, ok=%t", state, ok)
	}
}

func TestProdex04374LiveGoalRecoveryUsesNativeOverloadEvidence(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial live goal state was unavailable")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n")
	if closeErr := file.Close(); err != nil || closeErr != nil {
		t.Fatalf("append native overload: write=%v close=%v", err, closeErr)
	}
	state, ok := monitor.readState(t.Context())
	if !ok || state.Status != "usage_limited" || state.SessionID != id {
		t.Fatalf("native overload did not trigger recovery: %#v, ok=%t", state, ok)
	}
}

func TestProdex04377LiveGoalRecoveryRetriesTransientTailRead(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial live goal state was unavailable")
	}
	hidden := filepath.Join(root, "hidden.jsonl")
	if err := os.Rename(path, hidden); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, ok := monitor.readState(t.Context()); !ok || monitor.scanRetryAt.IsZero() {
		t.Fatalf("transient tail failure was not scheduled: ok=%t retry=%v", ok, monitor.scanRetryAt)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(hidden, path); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" {
		t.Fatalf("retry backoff was bypassed: %#v, ok=%t", state, ok)
	}
	monitor.scanRetryAt = time.Time{}
	state, ok := monitor.readState(t.Context())
	if !ok || state.Status != "usage_limited" {
		t.Fatalf("recovered tail after transient failure: %#v, ok=%t", state, ok)
	}
}

func TestProdex04374LiveGoalMonitorConsumesTurnsAndFiltersForeignThreads(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial live goal state was not readable")
	}
	appendRecord := func(raw string) {
		t.Helper()
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.WriteString(raw + "\n"); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	appendRecord(`{"type":"response_item","payload":{"type":"message","role":"user"}}`)
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" || !monitor.accepted {
		t.Fatalf("accepted turn was not retained: %#v, accepted=%t", state, monitor.accepted)
	}
	consumed := monitor.scanOffset
	appendRecord(`{"type":"task_complete","thread_id":"00000000-0000-4000-8000-000000000002","error":{"codex_error_info":"server_overloaded"}}`)
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" {
		t.Fatalf("foreign terminal event changed state: %#v, ok=%t", state, ok)
	}
	if monitor.scanOffset <= consumed {
		t.Fatalf("foreign event was not consumed: before=%d after=%d", consumed, monitor.scanOffset)
	}
	beforeFiller := monitor.scanOffset
	filler := strings.TrimSuffix(strings.Repeat("{}\n", int(recoveryScanCap04360/3)+100), "\n")
	appendRecord(filler)
	if _, ok := monitor.readState(t.Context()); !ok || monitor.scanOffset <= beforeFiller {
		t.Fatalf("first bounded scan did not advance: before=%d after=%d", beforeFiller, monitor.scanOffset)
	}
	firstFillerScan := monitor.scanOffset
	if _, ok := monitor.readState(t.Context()); !ok || monitor.scanOffset <= firstFillerScan {
		t.Fatalf("consumed offset did not reach filler tail: before=%d after=%d", firstFillerScan, monitor.scanOffset)
	}
	appendRecord(`{"type":"task_started","turn_id":"new-turn"}`)
	appendRecord(`{"type":"task_complete","turn_id":"new-turn","error":{"codex_error_info":"server_overloaded"}}`)
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" || monitor.accepted {
		t.Fatalf("new unaccepted turn reused old acceptance: %#v, accepted=%t", state, monitor.accepted)
	}
	appendRecord(`{"type":"response_item","payload":{"type":"message","role":"user"}}`)
	appendRecord(`{"type":"task_complete","turn_id":"final-turn","error":{"codex_error_info":"usage_limit_exceeded"}}`)
	state, ok := monitor.readState(t.Context())
	if !ok || state.Status != "usage_limited" || state.SessionID != id {
		t.Fatalf("accepted native terminal limit did not trigger recovery: %#v, ok=%t", state, ok)
	}
}

func TestProdex04377LiveGoalRecoveryTailsLargeRegularRollout(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	const largeOffset = int64(65 << 20)
	if err := file.Truncate(largeOffset); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("large rollout checkpoint was not accepted")
	}
	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"type":"response_item","payload":{"role":"user"}}
{"type":"error","error":{"code":"usage_limit_reached"}}
`); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	state, ok := monitor.readState(t.Context())
	if !ok || state.Status != "usage_limited" {
		t.Fatalf("large rollout tail did not trigger recovery: %#v, ok=%t", state, ok)
	}
}

func TestProdex04377LiveGoalRecoveryRebasesCompactedRolloutWithoutReplay(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	meta := `{"type":"session_meta","payload":{"id":"` + id + `"}}
`
	oldTurn := `{"type":"response_item","payload":{"role":"user"}}
{"type":"error","error":{"code":"usage_limit_reached"}}
`
	if err := os.WriteFile(path, []byte(meta+oldTurn), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial rollout state was unavailable")
	}
	if err := os.WriteFile(path, []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" {
		t.Fatalf("compacted historical evidence was replayed: %#v, ok=%t", state, ok)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"type":"response_item","payload":{"role":"user"}}
{"type":"error","error":{"code":"usage_limit_reached"}}
`); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	state, ok := monitor.readState(t.Context())
	if !ok || state.Status != "usage_limited" {
		t.Fatalf("new post-compaction evidence did not trigger recovery: %#v, ok=%t", state, ok)
	}
}

func TestProdex04377LiveGoalRecoverySeesCapacityAfterUnobservedCompaction(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	meta := `{"type":"session_meta","payload":{"id":"` + id + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(meta+strings.Repeat("{}\n", 1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial live goal state was unavailable")
	}
	compacted := meta +
		`{"type":"event_msg","payload":{"type":"context_compacted"}}` + "\n" +
		`{"type":"response_item","payload":{"role":"user"}}` + "\n" +
		`{"type":"error","error":{"code":"usage_limit_reached"}}` + "\n"
	if err := os.WriteFile(path, []byte(compacted), 0o600); err != nil {
		t.Fatal(err)
	}
	state, ok := monitor.readState(t.Context())
	if !ok || state.Status != "usage_limited" {
		t.Fatalf("unobserved compaction lost capacity evidence: %#v, ok=%t", state, ok)
	}
}

func TestProdex04377LiveGoalRecoveryRejectsSymlinkReplacementAndForeignSessionMeta(t *testing.T) {
	root := t.TempDir()
	id := "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	path := filepath.Join(root, "rollout-2026-10-10T10-00-00-"+id+".jsonl")
	foreign := filepath.Join(root, "foreign.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+id+`"}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok {
		t.Fatal("initial rollout state was unavailable")
	}
	if err := os.Rename(path, foreign); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if state, ok := monitor.readState(t.Context()); !ok || state.Status != "active" {
		t.Fatalf("symlink replacement changed state: %#v, ok=%t", state, ok)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"00000000-0000-4000-8000-000000000002"}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	foreignMonitor := &liveGoalRecovery04361{
		marker: marker,
		read: func(context.Context, string) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: id, Status: "active"}, true
		},
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: id, Path: path}, nil
		},
	}
	if _, ok := foreignMonitor.readState(t.Context()); !ok {
		t.Fatal("foreign rollout checkpoint was unavailable")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"type":"response_item","payload":{"role":"user"}}
{"type":"error","error":{"code":"usage_limit_reached"}}
`); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	state, ok := foreignMonitor.readState(t.Context())
	if !ok || state.Status != "active" {
		t.Fatalf("foreign session metadata changed state: %#v, ok=%t", state, ok)
	}
}
