package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
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
	if len(reports) != 1 || reports[0].ThreadName != "latest" || reports[0].CWD != "/wrong" || reports[0].ParentThreadID != "parent" || reports[0].Source != "subagent" || reports[0].ID != threadID {
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

func TestProdex04356ReaderRemembersLatestTurnModelAndReasoningEffort(t *testing.T) {
	var report sessionentity.Session
	for _, line := range []string{
		`{"timestamp":"2026-04-29T11:59:00Z","type":"session_meta","payload":{"id":"` + threadID + `"}}`,
		`{"timestamp":"2026-04-29T12:00:00Z","type":"turn_context","payload":{"model":"gpt-5.2-codex","effort":"medium"}}`,
		`{"timestamp":"2026-04-29T12:02:00Z","type":"turn_context","payload":{"model":"gpt-5.6-luna","effort":"max"}}`,
		`{"timestamp":"2026-04-29T12:03:00Z","type":"response_item","payload":{"model":"should-not-win","effort":"low"}}`,
	} {
		applySessionMetadata(&report, []byte(line))
	}
	if report.LastModel != "gpt-5.6-luna" || report.LastReasoningEffort != "max" {
		t.Fatalf("latest session settings = model:%q effort:%q", report.LastModel, report.LastReasoningEffort)
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

func TestReaderClassifiesCodex160SessionSources(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"cli", `"cli"`, "cli"},
		{"vscode", `"vscode"`, "vscode"},
		{"exec", `"exec"`, "exec"},
		{"mcp", `"mcp"`, "mcp"},
		{"custom", `{"custom":"desktop"}`, "custom"},
		{"internal", `{"internal":"guardian"}`, "internal"},
		{"subagent", `{"subagent":{"review":{}}}`, "subagent"},
		{"unknown", `"future-source"`, "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var report sessionentity.Session
			applySessionMetadata(&report, []byte(`{"type":"session_meta","payload":{"id":"`+threadID+`","source":`+test.source+`}}`))
			if report.Source != test.want {
				t.Fatalf("source %s = %q, want %q", test.source, report.Source, test.want)
			}
		})
	}
}

func TestReaderExtractsCodex160PreviewFromFirstUserMessage(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "sessions", "2026", "10", "05")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "rollout-2026-10-05T00-00-00-"+threadID+".jsonl")
	content := strings.Join([]string{
		`{"timestamp":"2026-10-05T00:00:00Z","type":"session_meta","payload":{"session_id":"` + threadID + `","id":"` + threadID + `","timestamp":"2026-10-05T00:00:00Z","cwd":"/repo","originator":"codex","cli_version":"0.160.0","source":"cli","model_provider":"openai","history_mode":"legacy"}}`,
		`{"timestamp":"2026-10-05T00:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"system prelude\n## My request for Codex:\nFix the picker","kind":"plain"}}`,
		`{"timestamp":"2026-10-05T00:00:02Z","type":"event_msg","payload":{"type":"user_message","message":"later message","kind":"plain"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	reports, err := NewReader().List(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].ID != threadID || reports[0].Source != "cli" ||
		reports[0].Preview != "Fix the picker" || reports[0].CWD != "/repo" || reports[0].ModelProvider != "openai" {
		t.Fatalf("Codex 0.160 report = %#v", reports)
	}
}

func TestSessionPreviewReadsResponseItemUserTextOnly(t *testing.T) {
	var report sessionentity.Session
	applySessionMetadata(&report, []byte(`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ignore"}]}}`))
	if report.Preview != "" {
		t.Fatalf("assistant preview = %q", report.Preview)
	}
	applySessionMetadata(&report, []byte(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first"},{"type":"input_image","image_url":"data:"},{"type":"input_text","text":"second"}]}}`))
	if report.Preview != "first\nsecond" {
		t.Fatalf("user preview = %q", report.Preview)
	}
}
