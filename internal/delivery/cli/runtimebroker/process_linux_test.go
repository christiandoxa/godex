//go:build linux

package runtimebroker

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestCleanupStaleLeasesRemovesZombieButKeepsLiveProcess(t *testing.T) {
	child := exec.Command("sh", "-c", "exit 0")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Wait() }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		state, err := linuxProcessState(child.Process.Pid)
		if errors.Is(err, os.ErrNotExist) {
			t.Skip("test child was reaped before its zombie state could be observed")
		}
		if err != nil {
			t.Fatal(err)
		}
		if state == "Z" {
			break
		}
		if time.Now().After(deadline) {
			t.Skipf("test child did not become a zombie (state %s)", state)
		}
		time.Sleep(time.Millisecond)
	}

	if !processAlive(os.Getpid()) {
		t.Fatal("current process was classified as absent")
	}
	if processAlive(child.Process.Pid) {
		t.Fatal("zombie process was classified as alive")
	}

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.CreateLease("broker", uint32(child.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	if got := store.CleanupStaleLeases("broker"); got != 0 {
		t.Fatalf("zombie lease count = %d, want 0", got)
	}
	if _, err := os.Stat(lease.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("zombie lease remains: %v", err)
	}
}

func TestLinuxProcessStateUsesFinalCommSeparator(t *testing.T) {
	state, err := linuxProcessStateFromStat("12 (name) with ) marker) Z 1 2 3")
	if err != nil || state != "Z" {
		t.Fatalf("parsed process state = %q, error = %v", state, err)
	}
}
