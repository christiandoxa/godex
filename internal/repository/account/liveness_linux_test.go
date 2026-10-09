//go:build linux

package account

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestZombieLockOwnerIsRecovered(t *testing.T) {
	store := newTestStore(t)
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})

	deadline := time.Now().Add(time.Second)
	for !linuxProcessZombie(pid) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !linuxProcessZombie(pid) {
		t.Fatal("child did not become a zombie")
	}

	lockPath := filepath.Join(store.Root(), "state.lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	ownerPath := filepath.Join(lockPath, "owner")
	if err := os.WriteFile(ownerPath, []byte(strconv.Itoa(pid)+"-synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := store.withLock(context.Background(), func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("zombie lock was not recovered")
	}
}
