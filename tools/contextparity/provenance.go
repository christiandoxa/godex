package main

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"os/exec"
	"runtime/debug"
	"strings"
)

// Each context-forwarding comparison must identify the exact clean source
// and compiled Godex binary. A versioned Prodex digest alone is not proof
// that a stale or locally modified candidate is the requested commit.
func verifyContextProvenance(input options) error {
	if err := verifyContextSource(input.prodexSource, oracleSourceCommit); err != nil {
		return fmt.Errorf("Prodex source: %w", err)
	}
	if err := verifyContextSource(input.godexSource, input.godexCommit); err != nil {
		return fmt.Errorf("Godex source: %w", err)
	}
	info, err := buildinfo.ReadFile(input.godex)
	if err != nil {
		return fmt.Errorf("read Godex executable metadata: %w", err)
	}
	return verifyContextBuild(info.Settings, input.godexCommit)
}

func verifyContextSource(root, commit string) error {
	if root == "" || len(commit) != 40 {
		return errors.New("source and full expected commit are required")
	}
	raw, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(raw)) != commit {
		return fmt.Errorf("source HEAD is not the expected commit %s", commit)
	}
	dirty, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=normal").Output()
	if err != nil {
		return fmt.Errorf("inspect worktree cleanliness: %w", err)
	}
	if len(dirty) != 0 {
		return errors.New("source worktree has uncommitted or untracked files")
	}
	return nil
}

func verifyContextBuild(settings []debug.BuildSetting, commit string) error {
	revision, modified := "", ""
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	if len(commit) != 40 || revision != commit || modified != "false" {
		return fmt.Errorf("Godex executable provenance mismatch: revision=%q modified=%q", revision, modified)
	}
	return nil
}
