package account

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCrashRecoveryRestoresRemovalBeforeStateCommit(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	state, err := store.readState()
	if err != nil {
		t.Fatal(err)
	}
	state.Accounts = nil
	state.ActiveAccountID = ""
	trash := store.accountDir(account.ID) + ".remove-test"
	if _, err := store.beginTransaction("remove", account.ID, trash, state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.stageRemoval(account.ID, trash); err != nil {
		t.Fatal(err)
	}
	// A new store models a command after an interrupted process.
	restarted := NewFileStore(store.Root())
	if _, err := restarted.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(restarted.CodexHome(account.ID), "auth.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.journalPath()); !os.IsNotExist(err) {
		t.Fatalf("journal remains: %v", err)
	}
}

func TestCrashRecoveryFinishesCommittedRemoval(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	state, err := store.readState()
	if err != nil {
		t.Fatal(err)
	}
	state.Accounts = nil
	state.ActiveAccountID = ""
	trash := store.accountDir(account.ID) + ".remove-test"
	if _, err := store.beginTransaction("remove", account.ID, trash, state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.stageRemoval(account.ID, trash); err != nil {
		t.Fatal(err)
	}
	if _, err := store.writeState(state); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(store.Root()).List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(trash); !os.IsNotExist(err) {
		t.Fatalf("trash remains: %v", err)
	}
}

func TestCrashRecoveryRestoresAuthentication(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	auth := filepath.Join(store.CodexHome(account.ID), "auth.json")
	old, err := os.ReadFile(auth)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.readState()
	if err != nil {
		t.Fatal(err)
	}
	state.Accounts[0].UpdatedAt = time.Unix(42, 0)
	backup := auth + ".backup-test"
	if _, err := store.beginTransaction("auth", account.ID, backup, state); err != nil {
		t.Fatal(err)
	}
	staged := stagedHome(t, store)
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.replaceAuthentication(account.ID, staged, backup); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(store.Root()).List(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(auth)
	if err != nil || string(restored) != string(old) {
		t.Fatalf("auth rollback = %v", err)
	}
}

func TestActiveProfileLeaseBlocksMutationAndOldLiveLockIsNotStolen(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	release, err := store.AcquireProfiles(context.Background(), []string{account.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Remove(context.Background(), account.ID); err == nil {
		t.Fatal("removed running profile")
	}
	if _, err := store.CommitLogin(context.Background(), account, stagedHome(t, store), false); err == nil {
		t.Fatal("replaced running credentials")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Root(), "state.lock")
	token, ok, err := tryAcquireLock(path)
	if err != nil || !ok {
		t.Fatalf("lock = %v", err)
	}
	defer releaseLock(path, token)
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if stale, err := store.lockIsStale(path); err != nil || stale {
		t.Fatalf("live lock stale = %v, %v", stale, err)
	}
}

func TestRecoveryDoesNotMistakeUnchangedMetadataForAuthCommit(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	auth := filepath.Join(store.CodexHome(account.ID), "auth.json")
	old, err := os.ReadFile(auth)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.readState()
	if err != nil {
		t.Fatal(err)
	}
	backup := auth + ".backup-test"
	if _, err := store.beginTransaction("auth", account.ID, backup, state); err != nil {
		t.Fatal(err)
	}
	staged := stagedHome(t, store)
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.replaceAuthentication(account.ID, staged, backup); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(store.Root()).List(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(auth)
	if err != nil || string(data) != string(old) {
		t.Fatalf("uncommitted auth was retained: %v", err)
	}
	if err := os.Remove(auth); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitLogin(context.Background(), account, stagedHome(t, store), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(auth); err != nil {
		t.Fatal("successful auth-only update with unchanged metadata was rolled back")
	}
}

func TestManagedAccountSymlinkCannotEscapeRepository(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	original := store.accountDir(account.ID)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Rename(original, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, original); err != nil {
		t.Skip(err)
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("managed account symlink accepted")
	}
	if _, err := store.AcquireProfiles(context.Background(), []string{account.ID}); err == nil {
		t.Fatal("external home leased")
	}
}
