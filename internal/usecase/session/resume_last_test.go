package session

import (
	"context"
	"errors"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
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
	}}}
	catalog := NewCatalog(accountsFake{}, reader, nil)
	catalog.SetSharedCodexHome("shared")
	for _, test := range []struct {
		args  []string
		model string
	}{
		{[]string{"exec", "resume", "--last"}, "exec-model"},
		{[]string{"resume", "--last", "--all"}, "other-model"},
	} {
		settings, err := catalog.LastResumeSettings(t.Context(), test.args)
		if err != nil || settings.LastModel != test.model {
			t.Fatalf("last settings model=%q error=%v", settings.LastModel, err)
		}
		if settings.ID != "" || settings.AccountID != "" || settings.UpstreamAccountID != "" || settings.CodexHome != "" {
			t.Fatal("settings claimed native selection or ownership")
		}
	}
	catalog.reader = readerFake{}
	if settings, err := catalog.LastResumeSettings(t.Context(), nil); err != nil || settings.LastModel != "" {
		t.Fatalf("empty native catalog failed: %v", err)
	}
	sentinel := errors.New("synthetic read failure")
	catalog.reader = lastReaderFailure{sentinel}
	if _, err := catalog.LastResumeSettings(t.Context(), nil); !errors.Is(err, sentinel) {
		t.Fatalf("lost storage failure: %v", err)
	}
}

type lastReaderFailure struct{ err error }

func (reader lastReaderFailure) List(context.Context, string) ([]sessionentity.Session, error) {
	return nil, reader.err
}
