package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const threadID = "00000000-0000-4000-8000-000000000001"

func TestReaderUsesRolloutMetadataAndLatestIndexName(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "sessions", "2026", "09", "30")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "rollout-synthetic.jsonl")
	content := `{"type":"session_meta","payload":{"id":"` + threadID + `","cwd":"/synthetic/project","model_provider":"openai","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent"}}}}}` + "\n" +
		`{"type":"response_item","payload":{"id":"ignored","title":"do not report conversation content","cwd":"/wrong"}}` + "\n" + strings.Repeat("x", maxSessionLineBytes+1)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	index := `{"id":"` + threadID + `","thread_name":"old"}` + "\n" + `{"id":"` + threadID + `","thread_name":"latest"}` + "\n"
	if err := os.WriteFile(filepath.Join(home, "session_index.jsonl"), []byte(index), 0600); err != nil {
		t.Fatal(err)
	}
	reports, err := NewReader().List(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].ThreadName != "latest" || reports[0].CWD != "/synthetic/project" || reports[0].ParentThreadID != "parent" || reports[0].ID != threadID {
		t.Fatalf("reports = %#v", reports)
	}
}

func TestReaderArchivedMissingMalformedAndCancellation(t *testing.T) {
	home := t.TempDir()
	reports, err := NewReader().List(context.Background(), home)
	if err != nil || len(reports) != 0 {
		t.Fatalf("empty = %#v, %v", reports, err)
	}
	dir := filepath.Join(home, "archived_sessions")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"rollout-bad.jsonl": "malformed\n", "rollout-unsafe.jsonl": `{"type":"session_meta","payload":{"id":"$(unsafe)"}}`, "rollout-valid.jsonl": `{"type":"session_meta","payload":{"id":"` + threadID + `"}}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	reports, err = NewReader().List(context.Background(), home)
	if err != nil || len(reports) != 1 {
		t.Fatalf("archived = %#v, %v", reports, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewReader().List(ctx, home); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestReaderRejectsSymlinkRootsAndSkipsSymlinkFiles(t *testing.T) {
	home, external := t.TempDir(), t.TempDir()
	if err := os.Symlink(external, filepath.Join(home, "sessions")); err != nil {
		t.Skip(err)
	}
	if _, err := NewReader().List(context.Background(), home); err == nil {
		t.Fatal("symlink root accepted")
	}
	if err := os.Remove(filepath.Join(home, "sessions")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(external, "rollout.jsonl")
	if err := os.WriteFile(target, []byte(`{"type":"session_meta","payload":{"id":"`+threadID+`"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "sessions", "rollout-linked.jsonl")); err != nil {
		t.Fatal(err)
	}
	reports, err := NewReader().List(context.Background(), home)
	if err != nil || len(reports) != 0 {
		t.Fatalf("symlink file = %#v, %v", reports, err)
	}
}

func TestCollectorRejectsFilesWhenBudgetIsAlreadyUsed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rollout-extra.jsonl"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := collectSessionPaths(context.Background(), dir, 0); err == nil {
		t.Fatal("archived file beyond total budget was ignored")
	}
}

func TestReaderExtractsCodexSessionSource(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "sessions")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "rollout-source.jsonl")
	content := `{"type":"session_meta","payload":{"id":"` + threadID + `","cwd":"/synthetic","source":"exec"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	reports, err := NewReader().List(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Source != "exec" {
		t.Fatalf("session source = %#v", reports)
	}
}
