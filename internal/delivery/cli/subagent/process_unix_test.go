//go:build unix

package subagent

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProdex04356SubAgentProcessGroupCleanupReachesDescendant(t *testing.T) {
	root := t.TempDir()
	pidFile := filepath.Join(root, "descendant.pid")
	command := exec.Command("sh", "-c", `sleep 30 & echo $! > "$1"; wait`, "sh", pidFile)
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer stopProcessTree(command)

	var descendant int
	deadline := time.Now().Add(2 * time.Second)
	for descendant == 0 && time.Now().Before(deadline) {
		content, err := os.ReadFile(pidFile)
		if err == nil {
			descendant, _ = strconv.Atoi(strings.TrimSpace(string(content)))
		}
		if descendant == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if descendant <= 0 {
		t.Fatal("descendant pid was not written")
	}
	if pgid, err := syscall.Getpgid(descendant); err != nil || pgid != command.Process.Pid {
		t.Fatalf("descendant pgid = %d, err = %v, want %d", pgid, err, command.Process.Pid)
	}
	stopProcessTree(command)
	_ = command.Wait()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(descendant, 0)
		if errors.Is(err, syscall.ESRCH) || processIsZombie(descendant) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("descendant remained alive after process-group cleanup")
}

func processIsZombie(pid int) bool {
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	_, rest, ok := strings.Cut(string(content), ") ")
	if !ok {
		return false
	}
	fields := strings.Fields(rest)
	return len(fields) > 0 && fields[0] == "Z"
}
