package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestValidateProbeRejectsCachedMaximumPromotion(t *testing.T) {
	item := probe{Name: "cache", Model: "gpt-6.1-sol", ContextWindow: "500000", CompactLimit: "430000"}
	got := childArgs{Model: item.Model, ContextWindow: "872000", CompactLimit: "784800"}
	if err := validateProbe(got, childArgs{Model: item.Model, ContextWindow: item.ContextWindow, CompactLimit: item.CompactLimit}); err == nil {
		t.Fatal("cached maximum promotion escaped the probe")
	}
}

func TestNegativeControlDetectsMutation(t *testing.T) {
	if err := checkNegativeControl(); err != nil {
		t.Fatal(err)
	}
}

func TestParseChildArgsKeepsLastExplicitValues(t *testing.T) {
	got := parseChildArgs([]string{
		"--model", "gpt-6.1-sol", "-c", "model_context_window=500000",
		"-cmodel_auto_compact_token_limit=430000", "-c", "model_context_window=700000",
	})
	want := childArgs{Model: "gpt-6.1-sol", ContextWindow: "700000", CompactLimit: "430000"}
	if got != want {
		t.Fatalf("parsed child args = %#v, want %#v", got, want)
	}
}

func TestProdex043710ContextParityRejectsDirtyAndStaleCandidate(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	good := []debug.BuildSetting{
		{Key: "vcs.revision", Value: commit},
		{Key: "vcs.modified", Value: "false"},
	}
	if err := verifyContextBuild(good, commit); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		settings []debug.BuildSetting
	}{
		{"dirty", []debug.BuildSetting{{Key: "vcs.revision", Value: commit}, {Key: "vcs.modified", Value: "true"}}},
		{"stale", []debug.BuildSetting{{Key: "vcs.revision", Value: strings.Repeat("0", 40)}, {Key: "vcs.modified", Value: "false"}}},
		{"missing", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := verifyContextBuild(tc.settings, commit); err == nil {
				t.Fatal("accepted an unqualified context probe executable")
			}
		})
	}
}

func TestProdex043710ContextParityRejectsDirtyAndWrongSource(t *testing.T) {
	root := t.TempDir()
	git := func(arguments ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, arguments...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "fixture.txt")
	git("-c", "user.name=Parity Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "synthetic")
	commit := git("rev-parse", "HEAD")
	if err := verifyContextSource(root, commit); err != nil {
		t.Fatal(err)
	}
	if err := verifyContextSource(root, strings.Repeat("0", 40)); err == nil {
		t.Fatal("accepted a different source revision")
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyContextSource(root, commit); err == nil {
		t.Fatal("accepted a dirty source tree")
	}
}
