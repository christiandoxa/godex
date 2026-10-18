package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

type workspaceReader interface {
	Workspaces(context.Context, string) ([]sessionentity.Workspace, error)
	WorkspaceAliases(context.Context, string) ([]string, error)
}

func (runner *Runner) SetWorkspaceTrust(reader workspaceReader, directory func() (string, error)) {
	runner.workspaces, runner.workspace = reader, directory
}

func (runner *Runner) trustedWorkspaceArguments(ctx context.Context, arguments []string) ([]string, error) {
	if runner.workspaces == nil {
		return arguments, nil
	}
	if runner.workspace == nil {
		return nil, errors.New("runtime working directory resolver is unavailable")
	}
	current, err := runner.workspace()
	if err != nil {
		return nil, fmt.Errorf("resolve Super working directory: %w", err)
	}
	workspaces := []string{current}
	if candidates, err := runner.workspaces.Workspaces(ctx, runner.sharedCodexHome); err == nil {
		workspaces = append(workspaces, sessionentity.RecentWorkspaces(candidates, 4096)...)
	}
	var entries []string
	seen := make(map[string]bool)
	encodedBytes := len("projects={}")
	for _, workspace := range workspaces {
		if !filepath.IsAbs(workspace) {
			continue
		}
		aliases, err := runner.workspaces.WorkspaceAliases(ctx, workspace)
		if err != nil {
			return nil, err
		}
		for _, candidate := range aliases {
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			quoted, _ := json.Marshal(candidate)
			entry := string(quoted) + `={trust_level="trusted"}`
			projected := encodedBytes + len(entry)
			if len(entries) > 0 {
				projected++
			}
			if len(entries) > 0 && projected > 128<<10 {
				return appendWorkspaceTrust(entries, arguments), nil
			}
			encodedBytes = projected
			entries = append(entries, entry)
		}
	}
	return appendWorkspaceTrust(entries, arguments), ctx.Err()
}

func appendWorkspaceTrust(entries, arguments []string) []string {
	if len(entries) == 0 {
		return arguments
	}
	return append([]string{"-c", "projects={" + strings.Join(entries, ",") + "}"}, arguments...)
}
