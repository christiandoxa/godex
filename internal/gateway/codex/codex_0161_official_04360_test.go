package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in exact upstream boundary smoke. The official 0.161.0 binary is placed
// outside the user's PATH and is never installed by this test. Every command
// ends at review/Codex argument validation before any provider model request.
// Set GODEX_TEST_CODEX_0161_BIN only after independent archive verification.
func TestProdex04360OfficialCodex0161CyberProgramParseBoundary(t *testing.T) {
	binary := os.Getenv("GODEX_TEST_CODEX_0161_BIN")
	if binary == "" {
		t.Skip("opt in with verified official Codex 0.161.0 binary")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 18*time.Second)
	defer cancel()
	home := t.TempDir()
	state := filepath.Join(home, "codex")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ program, want string }{
		{"standard", "not supported with"},
		{"daybreak_blue", "not supported with"},
		{"daybreak_red", "not supported with"},
		{"daybreak-blue", "invalid value"},
	} {
		t.Run(fixture.program, func(t *testing.T) {
			command := exec.CommandContext(ctx, binary, "exec", "--cyber-access-program", fixture.program, "review", "--uncommitted")
			command.Dir = home
			command.Env = []string{
				"HOME=" + home, "CODEX_HOME=" + state,
				"PATH=" + os.Getenv("PATH"), "NO_COLOR=1",
			}
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), fixture.want) {
				t.Fatalf("0.161.0 program %q boundary output=%s err=%v", fixture.program, output, err)
			}
			if ctx.Err() != nil {
				t.Fatalf("official CLI did not terminate at argument validation: %v", ctx.Err())
			}
		})
	}
}
