package codex

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunRuntimeDirectRepairsSelectedGoalBeforeChildStarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	root := t.TempDir()
	home := filepath.Join(root, "profile")
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	selector := "01900000-0000-7000-8000-000000000440"
	stale := filepath.Join(t.TempDir(), "deleted-overlay", "attachments", selector, "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("selected goal"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(shared, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec("INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)", selector, "read "+stale); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(writeProxyHelper(t, "exit 0"), Terminal{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
	process.SetSharedCodexHome(shared)
	if err := process.RunRuntimeDirect(context.Background(), home, []string{"resume", selector}, true); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(shared, "attachments", selector, "pasted-text-1.txt")
	if got := readGoalObjective(t, dbPath, selector); got != "read "+stable {
		t.Fatalf("selected goal objective = %q", got)
	}
	if got, err := os.ReadFile(stable); err != nil || string(got) != "selected goal" {
		t.Fatalf("stable selected attachment = %q, err=%v", got, err)
	}
}

func TestRepairResumeSessionBeforeLaunchRepairsExactGoalWithoutMaintenanceLock(t *testing.T) {
	root := t.TempDir()
	sessionID := "01900000-0000-7000-8000-000000000437"
	stale := filepath.Join(t.TempDir(), "deleted-overlay", "attachments", sessionID, "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec("INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)", sessionID, "read "+stale); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	release, acquired, err := (SessionLocker{}).TryLockCodexSessionsForMaintenance(root)
	if err != nil || !acquired {
		t.Fatalf("maintenance lock = acquired:%v err:%v", acquired, err)
	}
	defer release()
	if err := repairResumeSessionBeforeLaunch(root, root, []string{"resume", sessionID}); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, "attachments", sessionID, "pasted-text-1.txt")
	if got := readGoalObjective(t, dbPath, sessionID); got != "read "+stable {
		t.Fatalf("goal objective = %q", got)
	}
}

func TestRepairResumeSessionBeforeLaunchLeavesForeignGoalAndPickerAlone(t *testing.T) {
	root := t.TempDir()
	selected := "01900000-0000-7000-8000-000000000438"
	foreign := "01900000-0000-7000-8000-000000000439"
	stale := filepath.Join(t.TempDir(), "deleted-overlay", "attachments", foreign, "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "goals_1.sqlite")
	db := createGoalDB(t, dbPath)
	if _, err := db.Exec("INSERT INTO thread_goals (thread_id, objective) VALUES (?1, ?2)", foreign, "read "+stale); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	if err := repairResumeSessionBeforeLaunch(root, root, []string{"resume", selected}); err != nil {
		t.Fatal(err)
	}
	if err := repairResumeSessionBeforeLaunch(root, root, []string{"resume"}); err != nil {
		t.Fatal(err)
	}
	if got := readGoalObjective(t, dbPath, foreign); !strings.Contains(got, stale) {
		t.Fatalf("foreign goal changed: %q", got)
	}
}

func TestCodexResumeSessionIDAcceptsExplicitExecResume(t *testing.T) {
	const sessionID = "01900000-0000-7000-8000-000000000441"
	if got := codexResumeSessionID([]string{"exec", "resume", "-c", "model=auto", sessionID, "continue"}); got != sessionID {
		t.Fatalf("exec resume session ID = %q, want %q", got, sessionID)
	}
}
