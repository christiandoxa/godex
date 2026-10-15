//go:build linux

package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
	_ "modernc.org/sqlite"
)

const prodex043713DescriptorTestChild = "GODEX_043713_OBSERVER_CHILD"

// Lower RLIMIT_NOFILE only in a dedicated subprocess. The observer may not
// advance its unread cursor or request recovery under EMFILE; after pressure
// is released it must recover once without leaking descriptors on later polls.
func TestProdex043713LiveObserverDescriptorPressurePreservesUnreadCursor(t *testing.T) {
	if os.Getenv(prodex043713DescriptorTestChild) == "" {
		command := exec.Command(os.Args[0], "-test.run=^TestProdex043713LiveObserverDescriptorPressurePreservesUnreadCursor$", "-test.v")
		command.Env = append(os.Environ(), prodex043713DescriptorTestChild+"=1")
		output, err := command.CombinedOutput()
		if err != nil || !bytes.Contains(output, []byte("OBSERVER_EMFILE_RECOVERED")) {
			t.Fatalf("isolated low-descriptor observer regression: %v\n%s", err, output)
		}
		return
	}
	var original syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}
	if original.Cur < 96 {
		t.Skipf("original descriptor limit too low: %d", original.Cur)
	}
	root := t.TempDir()
	session := recovery04378Session
	path := filepath.Join(root, "rollout-"+session+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+session+`"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	marker := &sessionStartMarker04360{directory: root, path: filepath.Join(root, "session.id")}
	if err := os.WriteFile(marker.path, []byte(session+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "goals_1.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE thread_goals(thread_id TEXT PRIMARY KEY,status TEXT,updated_at_ms INTEGER); INSERT INTO thread_goals VALUES (?, 'blocked', 1)", session); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store := runtimerepo.NewGoalStateStore(root)
	monitor := &liveGoalRecovery04361{
		marker: marker, read: store.ReadGoalRecoveryState, observe: store.ObserveGoalRecoveryState,
		resolve: func(context.Context, string) (sessionmodel.Report, error) {
			return sessionmodel.Report{ID: session, Path: path}, nil
		},
	}
	if _, ok := monitor.readState(t.Context()); !ok || !monitor.checkpoint.valid {
		t.Fatal("initial observer did not establish checkpoint")
	}
	source, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.WriteString(`{"type":"event_msg","payload":{"type":"user_message"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"task_complete","error":{"codex_error_info":"server_overloaded"}}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	offset := monitor.scanOffset
	low := syscall.Rlimit{Cur: 96, Max: original.Max}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &low); err != nil {
		t.Fatal(err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_NOFILE, &original)
	var held []*os.File
	for {
		file, err := os.Open(os.DevNull)
		if err != nil {
			if !errors.Is(err, syscall.EMFILE) {
				t.Fatal(err)
			}
			break
		}
		held = append(held, file)
	}
	if _, ok := monitor.readState(t.Context()); ok || monitor.scanOffset != offset || monitor.sessionID != session ||
		!monitor.observerRetryAt.After(time.Now()) {
		t.Fatalf("descriptor pressure consumed terminal event or failed to schedule bounded retry: ok=%t offset=%d session=%s retry=%v",
			ok, monitor.scanOffset, monitor.sessionID, monitor.observerRetryAt)
	}
	for _, file := range held {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}
	if _, found := monitor.readState(t.Context()); found {
		t.Fatal("observer bypassed bounded retry backoff after EMFILE")
	}
	monitor.observerRetryAt = time.Now().Add(-time.Second)
	state, found := monitor.readState(t.Context())
	if !found || state.Status != "usage_limited" || monitor.scanOffset <= offset {
		t.Fatalf("unread failure not recovered after FD pressure: %+v found=%t offset=%d", state, found, monitor.scanOffset)
	}
	openFDs := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	base := openFDs()
	for i := 0; i < 128; i++ {
		monitor.readState(t.Context())
	}
	if count := openFDs(); count > base+2 {
		t.Fatalf("observer leaked FDs: before=%d after=%d", base, count)
	}
	t.Log("OBSERVER_EMFILE_RECOVERED")
}
