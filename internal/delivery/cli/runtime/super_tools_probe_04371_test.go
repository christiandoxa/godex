package runtime

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"
	"time"
)

func TestProdex04371SuperProbeKillsDescendantsHoldingOutputPipe(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	marker := filepath.Join(t.TempDir(), "descendant-alive")
	program := filepath.Join(t.TempDir(), "probe")
	script := "#!/bin/sh\n(sleep 1; printf alive > " + superResolverShellQuote(marker) + ") &\nwait\n"
	if err := os.WriteFile(program, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if output, ok := runSuperProbeCommand(ctx, program); ok || output != "" {
		t.Fatalf("timed-out probe = %q, %t", output, ok)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("probe reap took %s", elapsed)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant survived probe cancellation: stat error = %v", err)
	}
}
