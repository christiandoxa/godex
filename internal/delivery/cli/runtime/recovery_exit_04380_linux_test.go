//go:build linux

package runtime

import (
	"os/exec"
	"testing"
)

func TestProdex04380ActualUnixSigintExitSuppressesRelaunch(t *testing.T) {
	// Dedicated child self-signals, not the test runner. No real session is
	// touched; verify the os/exec signal status path matches Prodex 0.438.0.
	cmd := exec.Command("sh", "-c", "trap - INT; kill -INT $$")
	err := cmd.Run()
	if err == nil || !recoveryExitCancelled04360(err) {
		t.Fatalf("actual child SIGINT did not suppress recovery: %v", err)
	}
}
