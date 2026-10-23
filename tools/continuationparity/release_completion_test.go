package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseGateSuccessFollowsEverySelectedHTTPPhase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release shell gate runs on Linux")
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "scripts", "verify-release-parity.sh"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "# Native cached-token affinity")
	if start < 0 {
		t.Fatal("release gate lost final HTTP phases")
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "continuationparity"), []byte("#!/bin/sh\nexit \"$GATE_FIXTURE_EXIT\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"http-prompt-cache", "http-profile-status", "http-429-policy"} {
		for _, code := range []string{"0", "1"} {
			t.Run(phase+"/"+code, func(t *testing.T) {
				script := "set -euo pipefail\nphase=" + phase + "\ntmp=$GATE_FIXTURE_DIR\nroot=$GATE_FIXTURE_DIR\ngo() { return 0; }\ngit() { printf 'synthetic-sha'; }\n" + string(source[start:])
				command := exec.CommandContext(t.Context(), "bash", "-c", script)
				command.Env = append(os.Environ(), "GATE_FIXTURE_DIR="+directory, "GATE_FIXTURE_EXIT="+code)
				output, err := command.CombinedOutput()
				passed := strings.Contains(string(output), "PASS: Prodex")
				if (err == nil) != (code == "0") || passed != (code == "0") {
					t.Fatalf("exit=%v announced=%t want_success=%t output=%q", err, passed, code == "0", output)
				}
			})
		}
	}
}
