package codex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const officialCodex0163LinuxSHA04380 = "399ff69c431c066e40de318fba45c2b07f45a76b19939d247ee4828fc70c4215"

// These opt-in probes never change the installed Codex or create a model turn.
func TestProdex04380OfficialCodex0163AppServerInitializeAndThreadList(t *testing.T) {
	testOfficialCodexAppServer(t, "GODEX_TEST_CODEX_0163_APP_SERVER_BIN", "0.163.0", officialCodex0163LinuxSHA04380)
}

func TestProdex04390OfficialCodex0164AppServerInitializeAndThreadList(t *testing.T) {
	testOfficialCodexAppServer(t, "GODEX_TEST_CODEX_0164_APP_SERVER_BIN", "0.164.0", "b93f6811d3f653f0fb83be4050d857fb9abbb111dfa2ce160563327daff7f306")
}

func testOfficialCodexAppServer(t *testing.T, variable, version, expectedSHA string) {
	t.Helper()
	binary := os.Getenv(variable)
	if binary == "" {
		t.Skipf("set %s to the verified official Codex %s Linux binary", variable, version)
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("release qualification requires native Linux amd64")
	}
	file, err := os.Open(binary)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 512<<20 {
		_ = file.Close()
		t.Fatalf("official Codex artifact is not a bounded regular file: %v", statErr)
	}
	digest := sha256.New()
	_, err = io.Copy(digest, file)
	closeErr := file.Close()
	if err != nil || closeErr != nil || hex.EncodeToString(digest.Sum(nil)) != expectedSHA {
		t.Fatalf("Codex %s binary SHA mismatch: read=%v close=%v", version, err, closeErr)
	}
	versionOutput, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(versionOutput)) != "codex-cli "+version {
		t.Fatalf("unsupported native binary version=%q err=%v", versionOutput, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	root := t.TempDir()
	home := filepath.Join(root, "codex-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, "app-server", "--listen", "stdio://")
	command.Dir = root
	command.Env = []string{"HOME=" + root, "CODEX_HOME=" + home, "PATH=" + os.Getenv("PATH"), "NO_COLOR=1", "CI=1"}
	configureCodexAppServerProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		terminateCodexAppServerProcess(command)
		_ = command.Wait()
	}()
	if err := reconcileCodexThreadIndexProtocol(stdout, stdin); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("real %s handshake deadline expired", version)
		}
		t.Fatal(fmt.Errorf("actual %s initialize, thread/list active+archived failed: %w, stderr=%q", version, err, stderr.String()))
	}
	if ctx.Err() != nil {
		t.Fatalf("native handshake cancelled: %v", ctx.Err())
	}
}
