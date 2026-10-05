package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	authmodel "github.com/christiandoxa/godex/internal/model/auth"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
)

type fakeCurrentCodex struct {
	source        string
	insecure      bool
	err           error
	auth          string
	calls         int
	completeCalls int
	identity      authmodel.ImportCurrentIdentity
}

func (fake *fakeCurrentCodex) StageImportCurrentAuth(_ context.Context, source, staged string, insecure bool) (authmodel.ImportCurrentIdentity, error) {
	fake.calls++
	fake.source = source
	fake.insecure = insecure
	auth := fake.auth
	if auth == "" {
		auth = `{"synthetic":true}`
	}
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte(auth), 0o600); err != nil {
		return authmodel.ImportCurrentIdentity{}, err
	}
	if fake.err != nil {
		return authmodel.ImportCurrentIdentity{}, fake.err
	}
	identity := fake.identity
	if identity.Email == "" && identity.ChatGPTAccountID == "" {
		identity = authmodel.ImportCurrentIdentity{
			Email: "imported@example.com", ChatGPTAccountID: "imported-account",
		}
	}
	return identity, nil
}

func (fake *fakeCurrentCodex) CompleteImportCurrentHome(_ context.Context, source, staged string) error {
	fake.completeCalls++
	fake.source = source
	return os.WriteFile(filepath.Join(staged, "history.jsonl"), []byte("synthetic history"), 0o600)
}

func TestImportCurrentDefaultsNameActivatesAndRejectsExistingRequestedName(t *testing.T) {
	store := accountrepo.NewFileStore(t.TempDir())
	codex := &fakeCurrentCodex{}
	importer := NewImportCurrent(store, codex, "/synthetic/current-codex")

	first, err := importer.Run(context.Background(), authmodel.ImportCurrentRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "default" {
		t.Fatalf("default import name = %q", first.Name)
	}
	current, err := store.Current(context.Background())
	if err != nil || current.ID != first.ID {
		t.Fatalf("default active profile = %#v err=%v", current, err)
	}

	codex.identity = authmodel.ImportCurrentIdentity{
		Email: "second@example.com", ChatGPTAccountID: "second-account",
	}
	second, err := importer.Run(
		context.Background(), authmodel.ImportCurrentRequest{Name: "second"},
	)
	if err != nil {
		t.Fatal(err)
	}
	current, err = store.Current(context.Background())
	if err != nil || current.ID != second.ID {
		t.Fatalf("second import was not activated: %#v err=%v", current, err)
	}

	beforeCalls := codex.calls
	codex.identity = authmodel.ImportCurrentIdentity{
		Email: "third@example.com", ChatGPTAccountID: "third-account",
	}
	if _, err := importer.Run(
		context.Background(), authmodel.ImportCurrentRequest{Name: "second"},
	); err == nil {
		t.Fatal("existing requested profile name unexpectedly accepted")
	}
	if codex.calls != beforeCalls {
		t.Fatalf("name collision copied native home; import calls %d -> %d", beforeCalls, codex.calls)
	}
}

func TestImportCurrentCommitsManagedAccount(t *testing.T) {
	store := accountrepo.NewFileStore(t.TempDir())
	codex := &fakeCurrentCodex{}
	importer := NewImportCurrent(store, codex, "/synthetic/current-codex")
	importer.now = func() time.Time { return time.Unix(11, 0) }

	account, err := importer.Run(context.Background(), authmodel.ImportCurrentRequest{Name: "main", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if codex.source != "/synthetic/current-codex" || !codex.insecure || account.Name != "main" || account.Email != "imported@example.com" {
		t.Fatalf("import result = source %q insecure %t account %+v", codex.source, codex.insecure, account)
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != account.ID {
		t.Fatalf("current account = %+v", current)
	}
	history, err := os.ReadFile(filepath.Join(store.CodexHome(account.ID), "history.jsonl"))
	if err != nil || string(history) != "synthetic history" {
		t.Fatalf("committed history = %q, err = %v", history, err)
	}
}

func TestImportCurrentPreservesExistingAccountHome(t *testing.T) {
	store := accountrepo.NewFileStore(t.TempDir())
	codex := &fakeCurrentCodex{}
	importer := NewImportCurrent(store, codex, "/synthetic/current-codex")
	request := authmodel.ImportCurrentRequest{Name: "main"}
	account, err := importer.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(store.CodexHome(account.ID), "history.jsonl")
	if err := os.WriteFile(statePath, []byte("existing managed history"), 0o600); err != nil {
		t.Fatal(err)
	}
	codex.auth = `{"synthetic":"updated"}`
	updated, err := importer.Run(context.Background(), authmodel.ImportCurrentRequest{Name: "renamed"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "main" {
		t.Fatalf("duplicate import renamed existing account to %q", updated.Name)
	}
	content, err := os.ReadFile(statePath)
	if err != nil || string(content) != "existing managed history" {
		t.Fatalf("existing managed history = %q, err = %v", content, err)
	}
	content, err = os.ReadFile(filepath.Join(store.CodexHome(account.ID), "auth.json"))
	if err != nil || string(content) != codex.auth {
		t.Fatalf("updated auth = %q, err = %v", content, err)
	}
	if codex.completeCalls != 1 {
		t.Fatalf("duplicate identity performed full-home copy: complete calls = %d", codex.completeCalls)
	}
	accounts, err := store.List(context.Background())
	if err != nil || len(accounts) != 1 {
		t.Fatalf("managed accounts = %d, err = %v", len(accounts), err)
	}
}

func TestImportCurrentDuplicateIdentityDoesNotRequireFullNativeHomeCopy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlink setup requires Windows privileges")
	}
	store := accountrepo.NewFileStore(t.TempDir())
	existing, err := accountentity.NewAccount(
		accountentity.Identity{ChatGPTAccountID: "same-account"}, "primary", time.Unix(1, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.CreateStagedHome()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte(`{"synthetic":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "config.toml"), []byte("model = \"old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	existing, err = store.CommitLogin(context.Background(), existing, staged, true)
	if err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(store.CodexHome(existing.ID), "history.jsonl")
	if err := os.WriteFile(keep, []byte("existing history"), 0o600); err != nil {
		t.Fatal(err)
	}

	source := t.TempDir()
	if err := os.Chmod(source, 0o700); err != nil {
		t.Fatal(err)
	}
	authJSON := `{"auth_mode":"chatgpt","tokens":{"access_token":"opaque","account_id":"same-account"}}`
	if err := os.WriteFile(filepath.Join(source, "auth.json"), []byte(authJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".", filepath.Join(source, "directory-link")); err != nil {
		t.Fatal(err)
	}
	importer := NewImportCurrent(store, codex.NewCodexProcess("codex", codex.Terminal{}), source)

	result, err := importer.Run(context.Background(), authmodel.ImportCurrentRequest{Name: "duplicate"})
	if err != nil {
		t.Fatalf("duplicate identity unexpectedly required full native copy: %v", err)
	}
	if result.ID != existing.ID || result.Name != "primary" {
		t.Fatalf("duplicate result = %#v", result)
	}
	history, err := os.ReadFile(keep)
	if err != nil || string(history) != "existing history" {
		t.Fatalf("existing history = %q err=%v", history, err)
	}
	updatedAuth, err := os.ReadFile(filepath.Join(store.CodexHome(existing.ID), "auth.json"))
	if err != nil || string(updatedAuth) != authJSON {
		t.Fatalf("updated auth = %q err=%v", updatedAuth, err)
	}
}

func TestImportCurrentRemovesStagedHomeOnFailure(t *testing.T) {
	store := accountrepo.NewFileStore(t.TempDir())
	codex := &fakeCurrentCodex{err: errors.New("synthetic import failure")}
	importer := NewImportCurrent(store, codex, "/synthetic/current-codex")
	if _, err := importer.Run(context.Background(), authmodel.ImportCurrentRequest{}); err == nil {
		t.Fatal("failed import unexpectedly succeeded")
	}
	entries, err := os.ReadDir(store.TempDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("staged homes = %v, err = %v", entries, err)
	}
}

type staleDuplicateImportAccounts struct {
	root      string
	existing  accountentity.Account
	committed accountentity.Account
}

func (fake *staleDuplicateImportAccounts) CreateStagedHome() (string, error) {
	path := filepath.Join(fake.root, "staged")
	return path, os.MkdirAll(path, 0o700)
}

func (fake *staleDuplicateImportAccounts) RemoveStagedHome(path string) error {
	return os.RemoveAll(path)
}

func (fake *staleDuplicateImportAccounts) List(context.Context) ([]accountentity.Account, error) {
	// The usecase observes a duplicate identity here. Prodex holds its lifecycle
	// lock from this decision through commit, so that duplicate cannot disappear.
	return []accountentity.Account{fake.existing}, nil
}

func (fake *staleDuplicateImportAccounts) CommitImportCurrent(
	_ context.Context,
	candidate accountentity.Account,
	staged string,
	complete func() error,
) (accountentity.Account, error) {
	// Simulate the duplicate having been concurrently removed before commit.
	// A serialized commit must make the new-profile decision here and request
	// the full native home before persisting it.
	if complete != nil {
		if err := complete(); err != nil {
			return accountentity.Account{}, err
		}
	}
	if _, err := os.Stat(filepath.Join(staged, "history.jsonl")); err != nil {
		return accountentity.Account{}, errors.New("new import reached commit without full native home")
	}
	fake.committed = candidate
	return candidate, nil
}

func TestImportCurrentDuplicateDecisionCannotGoStaleBeforeCommit(t *testing.T) {
	existing, err := accountentity.NewAccount(
		accountentity.Identity{Email: "same@example.com", ChatGPTAccountID: "same-account"},
		"primary",
		time.Unix(1, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	accounts := &staleDuplicateImportAccounts{root: t.TempDir(), existing: existing}
	codex := &fakeCurrentCodex{identity: authmodel.ImportCurrentIdentity{
		Email: "same@example.com", ChatGPTAccountID: "same-account",
	}}
	importer := NewImportCurrent(accounts, codex, "/synthetic/current-codex")

	if _, err := importer.Run(context.Background(), authmodel.ImportCurrentRequest{Name: "new-profile"}); err != nil {
		t.Fatalf("import must not commit a partial new profile after duplicate state changes: %v", err)
	}
	if codex.completeCalls != 1 {
		t.Fatalf("full-home copy calls = %d, want 1 when commit creates a new profile", codex.completeCalls)
	}
}
