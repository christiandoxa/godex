package main

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"os/exec"
	"runtime/debug"
	"strings"
)

// WebSocket release observations must be attributable to a clean candidate,
// not a stale or locally patched Go executable.
func verifyWebSocketCandidate(binary, source, commit string) error {
	if strings.TrimSpace(source) == "" || len(commit) != 40 {
		return errors.New("--websocket requires --godex-source and a full --godex-commit")
	}
	head, err := exec.Command("git", "-C", source, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(head)) != commit {
		return fmt.Errorf("Godex source is not exact commit %s", commit)
	}
	status, err := exec.Command("git", "-C", source, "status", "--porcelain", "--untracked-files=normal").Output()
	if err != nil || len(status) != 0 {
		return errors.New("Godex source must be a clean committed worktree")
	}
	settings, err := readCandidateBuildSettings(binary)
	if err != nil {
		return err
	}
	return verifyCandidateVCS(settings, commit)
}

// Isolated to keep buildinfo dependency out of the no-VCS default HTTP probe.
func readCandidateBuildSettings(binary string) ([]debug.BuildSetting, error) {
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return nil, fmt.Errorf("read Godex candidate build metadata: %w", err)
	}
	return info.Settings, nil
}

func verifyCandidateVCS(settings []debug.BuildSetting, commit string) error {
	values := make(map[string]string, len(settings))
	for _, setting := range settings {
		values[setting.Key] = setting.Value
	}
	if values["vcs.revision"] != commit || values["vcs.modified"] != "false" {
		return fmt.Errorf("Godex executable VCS provenance mismatch: revision=%q dirty=%q",
			values["vcs.revision"], values["vcs.modified"])
	}
	return nil
}
