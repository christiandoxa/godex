//go:build linux

package codex

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	parentDeathModeEnv   = "GODEX_TEST_PARENT_DEATH_MODE"
	parentDeathPIDEnv    = "GODEX_TEST_PARENT_DEATH_PID"
	parentDeathConfigure = "configure"
	parentDeathHelper    = "helper"
	parentDeathChild     = "child"
)

// Prodex 0.437.3's session-owned companion has both process-group cleanup and
// a Linux parent-death guard. The unconfigured child is a negative control.
func TestProdex04373AppServerParentDeathGuard(t *testing.T) {
	for _, configured := range []bool{true, false} {
		name := "guarded"
		if !configured {
			name = "negative_control_without_guard"
		}
		t.Run(name, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "child.pid")
			helper := exec.Command(os.Args[0], "-test.run=^TestCodexAppServerParentDeathHelper$", "--")
			helper.Env = append(os.Environ(),
				parentDeathModeEnv+"="+parentDeathHelper,
				parentDeathPIDEnv+"="+pidFile,
			)
			if configured {
				helper.Env = append(helper.Env, parentDeathConfigure+"=1")
			}
			helper.Stdout, helper.Stderr = io.Discard, io.Discard
			if err := helper.Start(); err != nil {
				t.Fatal(err)
			}
			childPID := waitForParentDeathPID(t, pidFile)
			if err := helper.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = helper.Wait()

			if configured {
				waitForParentDeathExit(t, childPID)
				return
			}
			if !parentDeathProcessAlive(childPID) {
				t.Fatalf("negative control child %d exited when helper was killed", childPID)
			}
			_ = syscall.Kill(childPID, syscall.SIGKILL)
		})
	}
}

func TestCodexAppServerParentDeathHelper(t *testing.T) {
	if os.Getenv(parentDeathModeEnv) != parentDeathHelper {
		return
	}
	child := exec.Command(os.Args[0], "-test.run=^TestCodexAppServerParentDeathChild$", "--")
	child.Env = append(os.Environ(), parentDeathModeEnv+"="+parentDeathChild)
	if os.Getenv(parentDeathConfigure) == "1" {
		configureCodexAppServerProcess(child)
	}
	if err := child.Start(); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv(parentDeathPIDEnv), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		os.Exit(3)
	}
	select {}
}

func TestCodexAppServerParentDeathChild(t *testing.T) {
	if os.Getenv(parentDeathModeEnv) == parentDeathChild {
		select {}
	}
}

func waitForParentDeathPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if parseErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("parent-death helper did not publish child PID")
	return 0
}

func waitForParentDeathExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !parentDeathProcessAlive(pid) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("parent-death child %d survived helper termination", pid)
}

func parentDeathProcessAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	state, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	return err != nil || len(strings.Fields(string(state))) < 3 || strings.Fields(string(state))[2] != "Z"
}
