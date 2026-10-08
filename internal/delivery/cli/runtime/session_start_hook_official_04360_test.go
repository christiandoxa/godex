package runtime

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestProdex04360OfficialCodex0161TrustedSessionStartHookConfig(t *testing.T) {
	bin := os.Getenv("GODEX_TEST_CODEX_0161_BIN")
	if bin == "" {
		t.Skip("set GODEX_TEST_CODEX_0161_BIN to the official Codex 0.161.0 binary")
	}
	marker, err := newSessionStartMarker04360()
	if err != nil {
		t.Fatal(err)
	}
	defer marker.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := marker.codexHookArgumentsForOS04360(
		[]string{"exec-server", "--listen", "stdio"}, executable, "linux",
	)
	args = append([]string{"--strict-config"}, args...)
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "CODEX_HOME="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("official Codex 0.161.0 rejected the source-matched trusted hook config: %v: %s",
			err, string(output))
	}

}
