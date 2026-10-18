package runtime

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/pelletier/go-toml/v2"
)

type workspaceFixture struct {
	candidates []sessionentity.Workspace
	err        error
}

func (f workspaceFixture) Workspaces(context.Context, string) ([]sessionentity.Workspace, error) {
	return f.candidates, f.err
}
func (f workspaceFixture) WorkspaceAliases(ctx context.Context, path string) ([]string, error) {
	return []string{path, path}, ctx.Err()
}

func TestProdex04382SuperTrustPreservesSelectionAndUserArguments(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, `current\workspace`)
	directory := func() (string, error) { return current, nil }
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	runner := &Runner{}
	runner.SetWorkspaceTrust(workspaceFixture{candidates: []sessionentity.Workspace{{Path: a, ModifiedUnix: 1}, {Path: b, ModifiedUnix: 2}, {Path: a, ModifiedUnix: 3}, {Path: "relative", ModifiedUnix: 4}}}, directory)
	args := []string{"-c", `projects={"/user"={trust_level="untrusted"}}`, "exec", "synthetic"}
	got, err := runner.trustedWorkspaceArguments(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(args)+2 || got[0] != "-c" || !reflect.DeepEqual(got[2:], args) {
		t.Fatalf("arguments changed: %q", got)
	}
	var parsed struct {
		Projects map[string]struct {
			TrustLevel string `toml:"trust_level"`
		}
	}
	if err := toml.Unmarshal([]byte(got[1]), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Projects) != 3 || parsed.Projects[a].TrustLevel != "trusted" || parsed.Projects[b].TrustLevel != "trusted" || parsed.Projects[current].TrustLevel != "trusted" {
		t.Fatalf("projects=%+v", parsed.Projects)
	}
	if strings.Index(got[1], a) > strings.Index(got[1], b) {
		t.Fatal("recent workspace ordering changed")
	}
	runner.SetWorkspaceTrust(workspaceFixture{err: errors.New("synthetic unavailable history")}, directory)
	got, err = runner.trustedWorkspaceArguments(context.Background(), nil)
	if err != nil || len(got) != 2 {
		t.Fatalf("history failure lost current workspace: %q, %v", got, err)
	}
	parsed.Projects = nil
	if err := toml.Unmarshal([]byte(got[1]), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Projects) != 1 || parsed.Projects[current].TrustLevel != "trusted" {
		t.Fatalf("history failure changed current workspace: %+v", parsed.Projects)
	}
}

func TestProdex04382SuperTrustBoundsConfigurationAndEscapesPaths(t *testing.T) {
	runner := &Runner{}
	root := t.TempDir()
	quoted := filepath.Join(root, "quoted\"\n\x1b")
	runner.SetWorkspaceTrust(workspaceFixture{candidates: []sessionentity.Workspace{{Path: filepath.Join(root, strings.Repeat("x", 128<<10)), ModifiedUnix: 1}}}, func() (string, error) { return quoted, nil })
	got, err := runner.trustedWorkspaceArguments(context.Background(), nil)
	if err != nil || len(got) != 2 || len(got[1]) > 128<<10 {
		t.Fatalf("unbounded projects override: length=%d err=%v", len(got), err)
	}
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(got[1]), &parsed); err != nil {
		t.Fatalf("workspace path was not escaped as TOML: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runner.trustedWorkspaceArguments(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled trust preparation: %v", err)
	}
}

func TestProdex04382DirectSuperLaunchReceivesWorkspaceTrust(t *testing.T) {
	root := t.TempDir()
	process := &directSharedPreparingProcess{}
	runner := NewRunner(nil, process, nil)
	runner.SetSharedCodexHome(filepath.Join(root, "shared"))
	runner.SetWorkspaceTrust(workspaceFixture{}, func() (string, error) { return root, nil })
	if err := runner.RunDirectProfileWithOptions(t.Context(), filepath.Join(root, "profile"), []string{"exec", "synthetic"}, RuntimeLaunchOptions{SuperOverlay: true}); err != nil {
		t.Fatal(err)
	}
	if len(process.runArgs) != 4 || !strings.HasPrefix(process.runArgs[1], "projects={") || process.runArgs[2] != "exec" {
		t.Fatalf("direct Super arguments=%q", process.runArgs)
	}
	runner.SetWorkspaceTrust(workspaceFixture{}, func() (string, error) { return "", errors.New("synthetic cwd unavailable") })
	if _, err := runner.trustedWorkspaceArguments(t.Context(), nil); err == nil {
		t.Fatal("working-directory failure was hidden")
	}
}
