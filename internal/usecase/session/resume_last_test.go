package session

import (
	"context"
	"errors"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func TestLastResumeSettingsDoesNotClaimNativeSelection(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	reader := readerFake{map[string][]sessionentity.Session{"shared": {
		{ID: "old", CWD: cwd, Source: "cli", UpdatedUnix: 1, LastModel: "old"},
		{ID: "exec", CWD: cwd, Source: "exec", UpdatedUnix: 2,
			LastModel: "exec-model", LastReasoningEffort: "high", ModelProvider: "openai"},
		{ID: "archived", CWD: cwd, UpdatedUnix: 3, Path: "/shared/archived_sessions/rollout.jsonl"},
		{ID: "child", CWD: cwd, UpdatedUnix: 4, ParentThreadID: "exec"},
		{ID: "other", CWD: t.TempDir(), UpdatedUnix: 5, LastModel: "other-model"},
		{ID: "named", ThreadName: "continue", CWD: cwd, Source: "cli", LastModel: "name-only-model"},
	}}}
	catalog := NewCatalog(accountsFake{}, reader, nil)
	catalog.SetSharedCodexHome("shared")
	for _, test := range []struct {
		args     []string
		selector string
		model    string
	}{
		{[]string{"exec", "resume", "--last"}, "--last", "exec-model"},
		{[]string{"resume", "--last", "--all"}, "--last", "other-model"},
		{nil, "exec", "exec-model"},
		{nil, "ex", "exec-model"},
		{nil, "continue", ""},
		{nil, "missing", ""},
	} {
		settings, err := catalog.NativeResumeSettings(t.Context(), sessionmodel.Launch{SessionSelector: test.selector, Arguments: test.args})
		if err != nil || settings.LastModel != test.model {
			t.Fatalf("last settings model=%q error=%v", settings.LastModel, err)
		}
		if settings.ID != "" || settings.AccountID != "" || settings.UpstreamAccountID != "" || settings.CodexHome != "" {
			t.Fatal("settings claimed native selection or ownership")
		}
	}
	reader.reports["shared"] = append(reader.reports["shared"], sessionentity.Session{ID: "exec-other"})
	if _, err := catalog.NativeResumeSettings(t.Context(), sessionmodel.Launch{SessionSelector: "ex"}); err == nil {
		t.Fatal("ambiguous settings selector was accepted")
	}
	catalog.reader = readerFake{}
	if settings, err := catalog.NativeResumeSettings(t.Context(), sessionmodel.Launch{SessionSelector: "--last"}); err != nil || settings.LastModel != "" {
		t.Fatalf("empty native catalog failed: %v", err)
	}
	sentinel := errors.New("synthetic read failure")
	catalog.reader = lastReaderFailure{sentinel}
	if _, err := catalog.NativeResumeSettings(t.Context(), sessionmodel.Launch{SessionSelector: "--last"}); !errors.Is(err, sentinel) {
		t.Fatalf("lost storage failure: %v", err)
	}
	catalog.SetSharedCodexHome("")
	if _, err := catalog.NativeResumeSettings(t.Context(), sessionmodel.Launch{SessionSelector: "--last"}); err != nil {
		t.Fatalf("metadata lookup fell back to unrelated profiles: %v", err)
	}
}

type lastReaderFailure struct{ err error }

func (reader lastReaderFailure) List(context.Context, string) ([]sessionentity.Session, error) {
	return nil, reader.err
}
