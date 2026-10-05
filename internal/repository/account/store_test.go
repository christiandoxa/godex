package account

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	entity "github.com/christiandoxa/godex/internal/entity/account"
)

func TestCommitLoginDeduplicatesIdentity(t *testing.T) {
	store := newTestStore(t)
	first := newTestAccount(t, "work", "person@example.com", "account-1", time.Unix(1, 0))
	committed, err := store.CommitLogin(context.Background(), first, stagedHome(t, store), false)
	if err != nil {
		t.Fatal(err)
	}

	updated := newTestAccount(t, "ignored-default", "new@example.com", "account-1", time.Unix(2, 0))
	committedAgain, err := store.CommitLogin(context.Background(), updated, stagedHome(t, store), false)
	if err != nil {
		t.Fatal(err)
	}
	if committedAgain.ID != committed.ID || committedAgain.Name != "work" {
		t.Fatalf("deduplicated account = %#v", committedAgain)
	}
	accounts, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("account count = %d", len(accounts))
	}
}

func TestCommitLoginRejectsInvalidCandidateHomeAndContext(t *testing.T) {
	store := newTestStore(t)
	if _, err := store.CommitLogin(context.Background(), entity.Account{}, stagedHome(t, store), false); err == nil {
		t.Fatal("invalid account candidate unexpectedly accepted")
	}
	candidate := newTestAccount(t, "work", "work@example.com", "work-account", time.Unix(1, 0))
	if _, err := store.CommitLogin(context.Background(), candidate, filepath.Join(t.TempDir(), "outside"), false); err == nil {
		t.Fatal("staged home outside temp unexpectedly accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.CommitLogin(ctx, candidate, stagedHome(t, store), false); err == nil {
		t.Fatal("canceled login unexpectedly accepted")
	}
}

func TestStagedHomeLifecycleStaysUnderPrivateTempDirectory(t *testing.T) {
	store := newTestStore(t)
	staged, err := store.CreateStagedHome()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(store.TempDir(), staged)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		t.Fatalf("staged home path = %q", staged)
	}
	info, err := os.Stat(staged)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode(); mode.Perm() != 0o700 {
			t.Fatalf("staged home mode = %o", mode.Perm())
		}
	}
	if err := store.RemoveStagedHome(staged); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged home was not removed: %v", err)
	}
}

func TestDuplicateLoginDoesNotDiscardExistingMetadata(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	if _, err := store.SelectForLaunch(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	updated, err := entity.NewAccount(entity.Identity{ChatGPTAccountID: "account-1"}, "ignored", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitLogin(context.Background(), updated, stagedHome(t, store), false)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Email != first.Email || committed.ChatGPTAccountID != first.ChatGPTAccountID || committed.LastUsedAt.IsZero() {
		t.Fatalf("duplicate metadata = %#v", committed)
	}
}

func TestSelectForLaunchRotatesEnabledAccounts(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "one", "one@example.com", "account-1")
	second := commitTestAccount(t, store, "two", "two@example.com", "account-2")

	selected1, err := store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	selected2, err := store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	selected3, err := store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if selected1.ID != first.ID || selected2.ID != second.ID || selected3.ID != first.ID {
		t.Fatalf("selection order = %s, %s, %s", selected1.Name, selected2.Name, selected3.Name)
	}
}

func TestAddingEarlierSortedAccountDoesNotMoveExistingCursor(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "work", "one@example.com", "account-1")
	second := commitTestAccount(t, store, "admin", "two@example.com", "account-2")
	selected, err := store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != first.ID {
		t.Fatalf("selection after adding sorted account = %q", selected.ID)
	}
	selected, err = store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != second.ID {
		t.Fatalf("second selection = %q", selected.ID)
	}
}

func TestRemoveRepairsActiveSelection(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "one", "one@example.com", "account-1")
	second := commitTestAccount(t, store, "two", "two@example.com", "account-2")
	if _, err := store.SetActive(context.Background(), second.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Remove(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	selected, err := store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != first.ID {
		t.Fatalf("selected = %q", selected.ID)
	}
}

func TestStateNeverContainsAuthTokens(t *testing.T) {
	store := newTestStore(t)
	commitTestAccount(t, store, "one", "one@example.com", "account-1")
	content, err := os.ReadFile(store.statePath())
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(content))
	for _, forbidden := range []string{"access_token", "refresh_token", "id_token", "bearer"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("state contains %q", forbidden)
		}
	}
}

func TestReadStateRejectsUnsupportedVersion(t *testing.T) {
	store := newTestStore(t)
	content, err := json.Marshal(stateFile{Version: 999})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.statePath(), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestReadStateRejectsTrailingJSONValues(t *testing.T) {
	store := newTestStore(t)
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"version":1,"accounts":[]} {"version":1,"accounts":[]}`)
	if err := os.WriteFile(store.statePath(), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("trailing JSON unexpectedly accepted")
	}
}

func TestPrepareRejectsInvalidRootAndManagedDirectory(t *testing.T) {
	if err := NewFileStore(string(filepath.Separator)).Prepare(); err == nil {
		t.Fatal("filesystem root unexpectedly accepted")
	}
	if err := NewFileStore("relative-root").Prepare(); err == nil {
		t.Fatal("relative root unexpectedly accepted")
	}
	rootFile := filepath.Join(t.TempDir(), "root-file")
	if err := os.WriteFile(rootFile, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewFileStore(rootFile).Prepare(); err == nil {
		t.Fatal("file root unexpectedly accepted")
	}
	rootLink := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(t.TempDir(), rootLink); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := NewFileStore(rootLink).Prepare(); err == nil {
		t.Fatal("symbolic root unexpectedly accepted")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "accounts"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewFileStore(root).Prepare(); err == nil {
		t.Fatal("file accounts directory unexpectedly accepted")
	}
}

func TestReadStateRejectsDirectoryStatePath(t *testing.T) {
	store := newTestStore(t)
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.statePath(), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("directory state path unexpectedly accepted")
	}
}

func TestWriteStateReportsAtomicCommit(t *testing.T) {
	store := newTestStore(t)
	account := newTestAccount(t, "one", "one@example.com", "account-1", time.Unix(1, 0))
	committed, err := store.writeState(stateFile{Accounts: []entity.Account{account}})
	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("state write did not report commit")
	}
	if _, err := store.readState(); err != nil {
		t.Fatalf("read committed state: %v", err)
	}
}

func TestRemoveStagedHomeRejectsSymlinkEscape(t *testing.T) {
	store := newTestStore(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "keep")
	if err := os.WriteFile(target, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(store.TempDir(), "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := store.RemoveStagedHome(filepath.Join(link, "keep")); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("outside staging target: %v", err)
	}
}

func TestRemoveStagedHomeRejectsInvalidTargets(t *testing.T) {
	store := newTestStore(t)
	for _, path := range []string{"", filepath.Join(t.TempDir(), "outside")} {
		if err := store.RemoveStagedHome(path); err == nil {
			t.Fatalf("staged path %q unexpectedly accepted", path)
		}
	}
}

func TestValidateStagedHomeRejectsInvalidTargets(t *testing.T) {
	store := newTestStore(t)
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.TempDir(), "file")
	if err := os.WriteFile(file, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(store.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	for _, path := range []string{
		"",
		filepath.Join(t.TempDir(), "outside"),
		filepath.Join(store.TempDir(), "missing"),
		file,
		link,
	} {
		if err := store.validateStagedHome(path); err == nil {
			t.Fatalf("staged path %q unexpectedly accepted", path)
		}
	}
}

func TestRemoveStagedHomeAllowsMissingChild(t *testing.T) {
	store := newTestStore(t)
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(store.TempDir(), "missing", "home")
	if err := store.RemoveStagedHome(missing); err != nil {
		t.Fatalf("missing staged child: %v", err)
	}
}

func TestConcurrentSelectionsAreSerialized(t *testing.T) {
	store := newTestStore(t)
	commitTestAccount(t, store, "one", "one@example.com", "account-1")
	commitTestAccount(t, store, "two", "two@example.com", "account-2")

	var wait sync.WaitGroup
	errorsChannel := make(chan error, 20)
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.SelectForLaunch(context.Background(), "")
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	store := NewFileStore(t.TempDir())
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	return store
}

func stagedHome(t *testing.T, store *FileStore) string {
	t.Helper()
	path, err := os.MkdirTemp(store.TempDir(), "login-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "auth.json"), []byte(`{"synthetic":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.toml"), []byte("cli_auth_credentials_store = \"file\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestAccount(t *testing.T, name, email, accountID string, now time.Time) entity.Account {
	t.Helper()
	account, err := entity.NewAccount(entity.Identity{Email: email, ChatGPTAccountID: accountID}, name, now)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func commitTestAccount(t *testing.T, store *FileStore, name, email, accountID string) entity.Account {
	t.Helper()
	account := newTestAccount(t, name, email, accountID, time.Now())
	committed, err := store.CommitLogin(context.Background(), account, stagedHome(t, store), true)
	if err != nil {
		t.Fatal(err)
	}
	return committed
}

func TestCommitImportCurrentRejectsRequestedNameWithoutSuffixing(t *testing.T) {
	store := newTestStore(t)
	commitTestAccount(t, store, "default", "one@example.com", "account-1")
	candidate := newTestAccount(t, "default", "two@example.com", "account-2", time.Unix(2, 0))
	staged := stagedHome(t, store)

	if _, err := store.CommitImportCurrent(context.Background(), candidate, staged, nil); err == nil ||
		!strings.Contains(err.Error(), `profile "default" already exists`) {
		t.Fatalf("import-current name collision = %v", err)
	}
	accounts, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Name != "default" {
		t.Fatalf("accounts after collision = %#v", accounts)
	}
}

func TestCommitImportCurrentDeduplicatesAuthAndActivatesInSameCommit(t *testing.T) {
	store := newTestStore(t)
	primary := commitTestAccount(t, store, "primary", "one@example.com", "account-1")
	other := commitTestAccount(t, store, "other", "two@example.com", "account-2")
	if _, err := store.SetActive(context.Background(), other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetEnabled(context.Background(), primary.ID, false); err != nil {
		t.Fatal(err)
	}
	keepPath := filepath.Join(store.CodexHome(primary.ID), "history.jsonl")
	if err := os.WriteFile(keepPath, []byte("existing history"), 0o600); err != nil {
		t.Fatal(err)
	}
	staged := stagedHome(t, store)
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte(`{"synthetic":"fresh"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "history.jsonl"), []byte("copied history"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidate := newTestAccount(t, "duplicate", "one@example.com", "account-1", time.Unix(3, 0))

	completeCalled := false
	committed, err := store.CommitImportCurrent(context.Background(), candidate, staged, func() error {
		completeCalled = true
		return fmt.Errorf("duplicate identity must not complete full home")
	})
	if err != nil {
		t.Fatal(err)
	}
	if completeCalled {
		t.Fatal("duplicate identity completed full native home")
	}
	if committed.ID != primary.ID || committed.Name != "primary" || !committed.Enabled {
		t.Fatalf("committed duplicate = %#v", committed)
	}
	current, err := store.Current(context.Background())
	if err != nil || current.ID != primary.ID || !current.Enabled {
		t.Fatalf("active duplicate = %#v err=%v", current, err)
	}
	history, err := os.ReadFile(keepPath)
	if err != nil || string(history) != "existing history" {
		t.Fatalf("existing native state = %q err=%v", history, err)
	}
	auth, err := os.ReadFile(filepath.Join(store.CodexHome(primary.ID), "auth.json"))
	if err != nil || string(auth) != `{"synthetic":"fresh"}` {
		t.Fatalf("updated auth = %q err=%v", auth, err)
	}
}

func TestCommitLoginSuffixesCollidingDefaultNames(t *testing.T) {
	store := newTestStore(t)
	first := newTestAccount(t, "person", "person@one.example", "account-1", time.Unix(1, 0))
	second := newTestAccount(t, "person", "person@two.example", "account-2", time.Unix(2, 0))
	firstCommitted, err := store.CommitLogin(context.Background(), first, stagedHome(t, store), false)
	if err != nil {
		t.Fatal(err)
	}
	secondCommitted, err := store.CommitLogin(context.Background(), second, stagedHome(t, store), false)
	if err != nil {
		t.Fatal(err)
	}
	if firstCommitted.Name != "person" || secondCommitted.Name != "person-2" {
		t.Fatalf("names = %q, %q", firstCommitted.Name, secondCommitted.Name)
	}
}

func TestExplicitDuplicateNameIsRejected(t *testing.T) {
	store := newTestStore(t)
	commitTestAccount(t, store, "work", "one@example.com", "account-1")
	second := newTestAccount(t, "work", "two@example.com", "account-2", time.Unix(2, 0))
	if _, err := store.CommitLogin(context.Background(), second, stagedHome(t, store), true); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("duplicate name error = %v", err)
	}
	accounts, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("account count after rejected duplicate = %d", len(accounts))
	}
}

func TestSelectorsRejectAmbiguousExactMatches(t *testing.T) {
	store := newTestStore(t)
	commitTestAccount(t, store, "work", "one@example.com", "account-1")
	commitTestAccount(t, store, "other", "work", "account-2")

	if _, err := store.Resolve(context.Background(), "work"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("resolve error = %v", err)
	}
}

func TestRemoveActiveRepairsCursorToFirstEnabledAccount(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "one", "one@example.com", "account-1")
	second := commitTestAccount(t, store, "two", "two@example.com", "account-2")
	commitTestAccount(t, store, "three", "three@example.com", "account-3")
	if _, err := store.SetActive(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Remove(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	selected, err := store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != first.ID {
		t.Fatalf("selected after active removal = %q", selected.ID)
	}
}

func TestRemoveNonActiveAccountPreservesNextCursorTarget(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "one", "one@example.com", "account-1")
	second := commitTestAccount(t, store, "two", "two@example.com", "account-2")
	third := commitTestAccount(t, store, "three", "three@example.com", "account-3")
	selected, err := store.SelectForLaunch(context.Background(), "")
	if err != nil || selected.ID != first.ID {
		t.Fatalf("first selection = %q, %v", selected.ID, err)
	}
	if _, err := store.Remove(context.Background(), third.ID); err != nil {
		t.Fatal(err)
	}
	selected, err = store.SelectForLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != second.ID {
		t.Fatalf("next selection = %q", selected.ID)
	}
}

func TestManagedStateAndProfilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix owner/group permission bits")
	}
	store := newTestStore(t)
	account := commitTestAccount(t, store, "one", "one@example.com", "account-1")
	paths := []string{
		store.Root(),
		filepath.Join(store.Root(), "accounts"),
		store.TempDir(),
		store.statePath(),
		filepath.Dir(store.CodexHome(account.ID)),
		store.CodexHome(account.ID),
		filepath.Join(store.CodexHome(account.ID), "auth.json"),
		filepath.Join(store.CodexHome(account.ID), "config.toml"),
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		want := os.FileMode(0o700)
		if !info.IsDir() {
			want = 0o600
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s permissions = %o, want %o", path, got, want)
		}
	}
}

func TestReadStateRepairsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix owner/group permission bits")
	}
	store := newTestStore(t)
	commitTestAccount(t, store, "one", "one@example.com", "account-1")
	if err := os.Chmod(store.statePath(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.statePath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("state permissions = %o", got)
	}
}

func TestStateSymlinkIsRejected(t *testing.T) {
	store := newTestStore(t)
	stateContent := []byte(`{"version":1,"accounts":[]}`)
	target := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(target, stateContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.statePath()); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if _, err := store.List(context.Background()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("state symlink error = %v", err)
	}
}

func TestStaleLockRecoveryUsesAtomicTakeover(t *testing.T) {
	store := newTestStore(t)
	lockPath := filepath.Join(store.Root(), "state.lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-(staleLockAge + time.Second))
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := store.withLock(context.Background(), func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("stale lock was not recovered")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock remains after release: %v", err)
	}
}

func TestPromotionFailureRestoresExistingProfile(t *testing.T) {
	store := newTestStore(t)
	first := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	oldContent := []byte("old-profile")
	if err := os.WriteFile(filepath.Join(store.CodexHome(first.ID), "auth.json"), oldContent, 0o600); err != nil {
		t.Fatal(err)
	}

	staged := stagedHome(t, store)
	external := filepath.Join(t.TempDir(), "outside-token")
	if err := os.WriteFile(external, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(staged, "auth.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(staged, "auth.json")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	updated := newTestAccount(t, "ignored", "changed@example.com", "account-1", time.Unix(2, 0))
	if _, err := store.CommitLogin(context.Background(), updated, staged, false); err == nil {
		t.Fatal("expected unsafe profile rejection")
	}
	restored, err := os.ReadFile(filepath.Join(store.CodexHome(first.ID), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(oldContent) {
		t.Fatalf("restored profile = %q", restored)
	}
}

func TestStagedRemovalCanRestoreProfile(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	trash, restore, err := store.stageRemoval(account.ID, store.accountDir(account.ID)+".remove-test")
	if err != nil {
		t.Fatal(err)
	}
	if trash == "" {
		t.Fatal("expected staged removal path")
	}
	if _, err := os.Stat(store.accountDir(account.ID)); !os.IsNotExist(err) {
		t.Fatalf("profile still present while staged: %v", err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.CodexHome(account.ID)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(trash); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestConcurrentDistinctLoginsAreSerialized(t *testing.T) {
	store := newTestStore(t)
	const count = 8
	var wait sync.WaitGroup
	errorsChannel := make(chan error, count)
	for index := 0; index < count; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			account := newTestAccount(t, fmt.Sprintf("account-%d", index), fmt.Sprintf("person-%d@example.com", index), fmt.Sprintf("account-%d", index), time.Unix(int64(index+1), 0))
			_, err := store.CommitLogin(context.Background(), account, stagedHome(t, store), true)
			errorsChannel <- err
		}(index)
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
	accounts, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != count {
		t.Fatalf("account count = %d, want %d", len(accounts), count)
	}
}

func TestStateRejectsDuplicateIdentityMetadata(t *testing.T) {
	store := newTestStore(t)
	first := newTestAccount(t, "one", "person@example.com", "account-1", time.Unix(1, 0))
	second, err := entity.NewAccount(entity.Identity{Email: "person@example.com"}, "two", time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	state := stateFile{Version: stateVersion, Accounts: []entity.Account{first, second}}
	content, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.statePath(), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err == nil || !strings.Contains(err.Error(), "duplicate account identity") {
		t.Fatalf("duplicate identity error = %v", err)
	}
}

func TestStateAllowsDistinctChatGPTAccountsWithSameEmail(t *testing.T) {
	store := newTestStore(t)
	first := newTestAccount(t, "one", "person@example.com", "account-1", time.Unix(1, 0))
	second := newTestAccount(t, "two", "person@example.com", "account-2", time.Unix(2, 0))
	content, err := json.Marshal(stateFile{Version: stateVersion, Accounts: []entity.Account{first, second}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.statePath(), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEmailOnlyDuplicateIsRejectedWhenEmailHasMultipleAccounts(t *testing.T) {
	store := newTestStore(t)
	commitTestAccount(t, store, "one", "person@example.com", "account-1")
	commitTestAccount(t, store, "two", "person@example.com", "account-2")
	candidate, err := entity.NewAccount(entity.Identity{Email: "person@example.com"}, "three", time.Unix(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitLogin(context.Background(), candidate, stagedHome(t, store), false); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous email-only login error = %v", err)
	}
}

func TestRemoveProfileCanRetainManagedHome(t *testing.T) {
	store := newTestStore(t)
	account := commitTestAccount(t, store, "work", "person@example.com", "account-1")
	home := store.CodexHome(account.ID)
	if _, err := store.RemoveProfile(context.Background(), account.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); err != nil {
		t.Fatalf("retained profile home: %v", err)
	}
	accounts, err := store.List(context.Background())
	if err != nil || len(accounts) != 0 {
		t.Fatalf("accounts = %+v, err = %v", accounts, err)
	}
}

func TestCommitImportCurrentCompletesNewHomeWhileStateIsSerialized(t *testing.T) {
	store := newTestStore(t)
	existing := commitTestAccount(t, store, "existing", "existing@example.com", "existing-account")
	candidate := newTestAccount(t, "imported", "imported@example.com", "imported-account", time.Unix(9, 0))
	staged := stagedHome(t, store)
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte(`{"synthetic":"fresh"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	removeDone := make(chan error, 1)
	commitDone := make(chan error, 1)
	go func() {
		_, err := store.CommitImportCurrent(context.Background(), candidate, staged, func() error {
			close(callbackStarted)
			<-releaseCallback
			return os.WriteFile(filepath.Join(staged, "history.jsonl"), []byte("copied history"), 0o600)
		})
		commitDone <- err
	}()
	<-callbackStarted
	go func() {
		_, err := store.Remove(context.Background(), existing.ID)
		removeDone <- err
	}()

	select {
	case err := <-removeDone:
		t.Fatalf("profile mutation escaped import-current state lock: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(releaseCallback)
	if err := <-commitDone; err != nil {
		t.Fatal(err)
	}
	if err := <-removeDone; err != nil {
		t.Fatal(err)
	}
	accounts, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Name != "imported" {
		t.Fatalf("serialized import accounts = %#v", accounts)
	}
	history, err := os.ReadFile(filepath.Join(store.CodexHome(accounts[0].ID), "history.jsonl"))
	if err != nil || string(history) != "copied history" {
		t.Fatalf("committed native history = %q err=%v", history, err)
	}
}
