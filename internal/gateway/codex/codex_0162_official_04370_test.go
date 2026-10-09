package codex

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in credential-free qualification against the SHA-256-verified
// official Codex 0.162.0 app-server binary. This checks real wire RPCs
// without launching a model turn or reading the user's CODEX_HOME.
func TestProdex04370OfficialCodex0162AppServerThreadIndexBoundary(t *testing.T) {
	binary := os.Getenv("GODEX_TEST_CODEX_0162_APP_SERVER_BIN")
	if binary == "" {
		t.Skip("set GODEX_TEST_CODEX_0162_APP_SERVER_BIN to the verified official 0.162.0 binary")
	}
	version, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(version)) != "codex-app-server 0.162.0" {
		t.Fatalf("expected exact 0.162.0 app-server binary: output=%q error=%v", version, err)
	}
	contextWithDeadline, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	home := t.TempDir()
	codexHome := filepath.Join(home, "codex")
	if err := os.Mkdir(codexHome, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(contextWithDeadline, binary, "--listen", "stdio://")
	command.Dir = home
	command.Env = []string{
		"HOME=" + home, "CODEX_HOME=" + codexHome,
		"PATH=" + os.Getenv("PATH"), "NO_COLOR=1",
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	configureCodexAppServerProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		terminateCodexAppServerProcess(command)
		_ = command.Wait()
	}()
	if err := reconcileCodexThreadIndexProtocol(stdout, stdin); err != nil {
		t.Fatalf("actual Codex 0.162.0 thread/list active and archived RPC incompatible: %v; stderr=%q", err, stderr.String())
	}
	if contextWithDeadline.Err() != nil {
		t.Fatalf("app-server did not finish credential-free protocol within bound: %v", contextWithDeadline.Err())
	}
}
