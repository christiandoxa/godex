package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

func TestBundleImportRecoveryRollsBackInReverseAndRestoresSelections(t *testing.T) {
	ctx := context.Background()
	profileRoot := t.TempDir()
	profiles := profilerepo.NewStore(profileRoot)
	accounts := accountrepo.NewFileStore(t.TempDir())
	first := addLifecycleTestAccount(t, accounts, "first", "previous-auth")
	second := addLifecycleTestAccount(t, accounts, "second", "second-before")
	if _, err := accounts.SetActive(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	work := profileentity.Profile{Name: "work", CodexHome: profiles.ManagedHome("work"), Managed: true, Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI}}
	if err := profiles.ImportOpenAI(ctx, work, []byte("profile-before"), true); err != nil {
		t.Fatal(err)
	}

	const id = "0123456789abcdef0123456789abcdef"
	if err := profiles.PrepareBundleImportRollback(ctx, work.Name, id, []string{"auth.json"}); err != nil {
		t.Fatal(err)
	}
	if err := accounts.PrepareImportedAuthRollback(ctx, second.ID, id); err != nil {
		t.Fatal(err)
	}
	created := profileentity.Profile{Name: "new", CodexHome: profiles.ManagedHome("new"), Managed: true, Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI}}
	workBefore := importLifecycleProfile(work)
	openAI := func(name, auth string) profilemodel.ExportedProfile {
		return profilemodel.ExportedProfile{Name: name, Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: auth}
	}
	journal := profilemodel.ImportLifecycleJournal{
		Version: 1, ID: id, Phase: "applying", PreviousProfileActive: "work", PreviousAccountActive: first.ID,
		Actions: []profilemodel.ImportLifecycleAction{
			{
				Name: work.Name, Before: &workBefore, After: workBefore, BackupID: id,
				Files: importLifecycleFiles(openAI("work", "profile-after")),
			},
			{
				Name: second.Name, AccountID: second.ID, After: importLifecycleProfile(accountProfile(second, accounts.CodexHome(second.ID))), BackupID: id,
				Files: importLifecycleFiles(openAI(second.Name, "second-after")),
			},
			{
				Name: created.Name, Create: true, After: importLifecycleProfile(created),
				Files: importLifecycleFiles(openAI(created.Name, "created-auth")),
			},
		},
	}
	if err := profiles.WriteBundleImportJournal(journal); err != nil {
		t.Fatal(err)
	}
	journalBytes, err := os.ReadFile(filepath.Join(profileRoot, ".profile-import-journals", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"profile-before", "profile-after", "second-before", "second-after", "created-auth"} {
		if strings.Contains(string(journalBytes), secret) {
			t.Fatalf("journal contains secret %q", secret)
		}
	}
	if err := profiles.ReplaceAuth(ctx, work.Name, []byte("profile-after")); err != nil {
		t.Fatal(err)
	}
	if err := accounts.ReplaceImportedAuth(ctx, second.ID, []byte("second-after")); err != nil {
		t.Fatal(err)
	}
	if err := profiles.ImportBundleProfile(ctx, created, map[string][]byte{"auth.json": []byte("created-auth")}, id); err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.SetActive(ctx, created.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.SetActive(ctx, second.ID); err != nil {
		t.Fatal(err)
	}

	var order []string
	observedProfiles := &lifecycleOrderRepository{Store: profiles, order: &order}
	observedAccounts := &lifecycleOrderAccounts{FileStore: accounts, order: &order}
	catalog := NewCatalog(observedProfiles, observedAccounts, "")
	if _, err := catalog.List(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"remove:new", "account:" + second.ID, "profile:work"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("rollback order = %v, want %v", order, want)
	}
	assertProfileAuth(t, profiles, work.CodexHome, "profile-before")
	assertAccountAuth(t, accounts, second.ID, "second-before")
	if _, err := profiles.Resolve(ctx, created.Name); err == nil {
		t.Fatal("partially imported profile remains")
	}
	current, err := profiles.Current(ctx)
	if err != nil || current.Name != work.Name {
		t.Fatalf("active profile = %q, err=%v", current.Name, err)
	}
	activeAccount, err := accounts.ActiveID(ctx)
	if err != nil || activeAccount != first.ID {
		t.Fatalf("active account = %q, err=%v", activeAccount, err)
	}
	if journals, err := profiles.BundleImportJournals(); err != nil || len(journals) != 0 {
		t.Fatalf("remaining journals = %+v, err=%v", journals, err)
	}
}

func TestBundleImportPreparingAndCommittedJournalRecovery(t *testing.T) {
	for _, test := range []struct {
		phase string
		want  string
	}{
		{phase: "preparing", want: "before"},
		{phase: "committed", want: "after"},
	} {
		t.Run(test.phase, func(t *testing.T) {
			ctx := context.Background()
			repo := profilerepo.NewStore(t.TempDir())
			value := profileentity.Profile{Name: "work", CodexHome: repo.ManagedHome("work"), Managed: true, Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI}}
			if err := repo.ImportOpenAI(ctx, value, []byte("before"), true); err != nil {
				t.Fatal(err)
			}
			const id = "abcdef0123456789abcdef0123456789"
			before := importLifecycleProfile(value)
			if err := repo.PrepareBundleImportRollback(ctx, value.Name, id, []string{"auth.json"}); err != nil {
				t.Fatal(err)
			}
			if test.phase == "committed" {
				if err := repo.ReplaceAuth(ctx, value.Name, []byte("after")); err != nil {
					t.Fatal(err)
				}
			}
			journal := profilemodel.ImportLifecycleJournal{
				Version: 1, ID: id, Phase: test.phase,
				Actions: []profilemodel.ImportLifecycleAction{{
					Name: value.Name, Before: &before, After: before, BackupID: id,
					Files: importLifecycleFiles(profilemodel.ExportedProfile{Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "after"}),
				}},
			}
			if err := repo.WriteBundleImportJournal(journal); err != nil {
				t.Fatal(err)
			}
			catalog := NewCatalog(repo, &fakeAccounts{}, "")
			if _, err := catalog.List(ctx); err != nil {
				t.Fatal(err)
			}
			assertProfileAuth(t, repo, value.CodexHome, test.want)
			if journals, err := repo.BundleImportJournals(); err != nil || len(journals) != 0 {
				t.Fatalf("remaining journals = %+v, err=%v", journals, err)
			}
			if _, err := os.Lstat(filepath.Join(value.CodexHome, ".godex-import-backup-"+id)); !os.IsNotExist(err) {
				t.Fatalf("rollback backup remains: %v", err)
			}
		})
	}
}

func TestBundleImportApplyingInferenceKeepsCommittedExistingProfileAuth(t *testing.T) {
	ctx := context.Background()
	profiles := profilerepo.NewStore(t.TempDir())
	work := profileentity.Profile{
		Name: "work", CodexHome: profiles.ManagedHome("work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := profiles.ImportOpenAI(ctx, work, []byte("before"), true); err != nil {
		t.Fatal(err)
	}
	const id = "11111111111111111111111111111111"
	if err := profiles.PrepareBundleImportRollback(ctx, work.Name, id, []string{"auth.json"}); err != nil {
		t.Fatal(err)
	}
	before := importLifecycleProfile(work)
	journal := profilemodel.ImportLifecycleJournal{
		Version: 1, ID: id, Phase: "applying", PreviousProfileActive: work.Name,
		Actions: []profilemodel.ImportLifecycleAction{{
			Name: work.Name, Before: &before, After: before, BackupID: id,
			Files: importLifecycleFiles(profilemodel.ExportedProfile{
				Name: work.Name, Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "after",
			}),
		}},
	}
	if err := profiles.WriteBundleImportJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := profiles.ReplaceAuth(ctx, work.Name, []byte("after")); err != nil {
		t.Fatal(err)
	}

	catalog := NewCatalog(profiles, &fakeAccounts{}, "")
	if _, err := catalog.List(ctx); err != nil {
		t.Fatal(err)
	}
	assertProfileAuth(t, profiles, work.CodexHome, "after")
	if journals, err := profiles.BundleImportJournals(); err != nil || len(journals) != 0 {
		t.Fatalf("remaining journals = %+v, err=%v", journals, err)
	}
	if _, err := os.Lstat(filepath.Join(work.CodexHome, ".godex-import-backup-"+id)); !os.IsNotExist(err) {
		t.Fatalf("committed profile backup remains: %v", err)
	}
}

func TestBundleImportApplyingInferenceKeepsCommittedExistingAccountAuth(t *testing.T) {
	ctx := context.Background()
	profiles := profilerepo.NewStore(t.TempDir())
	accounts := accountrepo.NewFileStore(t.TempDir())
	account := addLifecycleTestAccount(t, accounts, "work", "before")
	if _, err := accounts.SetActive(ctx, account.ID); err != nil {
		t.Fatal(err)
	}
	const id = "22222222222222222222222222222222"
	if err := accounts.PrepareImportedAuthRollback(ctx, account.ID, id); err != nil {
		t.Fatal(err)
	}
	journal := profilemodel.ImportLifecycleJournal{
		Version: 1, ID: id, Phase: "applying", PreviousAccountActive: account.ID,
		Actions: []profilemodel.ImportLifecycleAction{{
			Name: account.Name, AccountID: account.ID,
			After: importLifecycleProfile(accountProfile(account, accounts.CodexHome(account.ID))), BackupID: id,
			Files: importLifecycleFiles(profilemodel.ExportedProfile{
				Name: account.Name, Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "after",
			}),
		}},
	}
	if err := profiles.WriteBundleImportJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := accounts.ReplaceImportedAuth(ctx, account.ID, []byte("after")); err != nil {
		t.Fatal(err)
	}

	catalog := NewCatalog(profiles, accounts, "")
	if _, err := catalog.List(ctx); err != nil {
		t.Fatal(err)
	}
	assertAccountAuth(t, accounts, account.ID, "after")
	if journals, err := profiles.BundleImportJournals(); err != nil || len(journals) != 0 {
		t.Fatalf("remaining journals = %+v, err=%v", journals, err)
	}
	if err := accounts.CleanupImportedAuthRollback(ctx, account.ID, id); err != nil {
		t.Fatalf("committed account backup cleanup was not idempotent: %v", err)
	}
}

func TestBundleImportCommitMarkerFailureRecoversAccordingToPersistedPhase(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		t.Run(map[bool]string{false: "applying", true: "committed"}[persisted], func(t *testing.T) {
			ctx := context.Background()
			store := profilerepo.NewStore(t.TempDir())
			repo := &commitMarkerFailureRepository{Store: store, persist: persisted}
			catalog := NewCatalog(repo, &fakeAccounts{}, "")
			catalog.SetAuthInspector(bundleInspector{identities: map[string]accountentity.Identity{
				"auth-next": {Email: "next@example.test", ChatGPTAccountID: "workspace-next"},
			}})
			path := filepath.Join(privateTempDir(t), "commit-marker.json")
			writeBundleFixture(t, store, path, "work", "auth-next")
			if _, err := catalog.Import(ctx, profilemodel.ImportRequest{Path: path}); err == nil {
				t.Fatal("injected commit marker failure was ignored")
			}
			reports, err := catalog.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(reports); got != 1 {
				t.Fatalf("profiles after recovery = %+v", reports)
			}
			if reports[0].Profile.Name != "work" {
				t.Fatalf("recovered committed profile = %+v", reports[0])
			}
			if journals, err := store.BundleImportJournals(); err != nil || len(journals) != 0 {
				t.Fatalf("remaining journals = %+v, err=%v", journals, err)
			}
		})
	}
}

func TestBundleImportWithoutSourceActiveProfileStaysInactive(t *testing.T) {
	ctx := context.Background()
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, "")
	catalog.SetAuthInspector(bundleInspector{identities: map[string]accountentity.Identity{
		"auth-work": {Email: "person@example.test", ChatGPTAccountID: "acct-work"},
	}})
	path := filepath.Join(privateTempDir(t), "inactive-import.json")
	payload := profilemodel.BundlePayload{
		ExportedAt: "2026-10-03T00:00:00Z", SourceProdexVersion: "0.435.1",
		Profiles: []profilemodel.ExportedProfile{{
			Name: "work", Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "auth-work",
		}},
	}
	content, err := repo.EncodeBundle(payload, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WriteBundle(path, content); err != nil {
		t.Fatal(err)
	}
	result, err := catalog.Import(ctx, profilemodel.ImportRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActiveProfile != "" {
		t.Fatalf("import active profile = %q, want none", result.ActiveProfile)
	}
	if active, err := repo.HasActive(ctx); err != nil || active {
		t.Fatalf("stored active profile = %t, err=%v", active, err)
	}
}

func TestActiveStandaloneRecoversInterruptedBundleImport(t *testing.T) {
	ctx := context.Background()
	profiles := profilerepo.NewStore(t.TempDir())
	work := profileentity.Profile{
		Name: "work", CodexHome: profiles.ManagedHome("work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := profiles.ImportOpenAI(ctx, work, []byte("before"), true); err != nil {
		t.Fatal(err)
	}
	created := profileentity.Profile{
		Name: "new", CodexHome: profiles.ManagedHome("new"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	const id = "0123456789abcdef0123456789abcdef"
	journal := profilemodel.ImportLifecycleJournal{
		Version: 1, ID: id, Phase: "applying", PreviousProfileActive: work.Name,
		Actions: []profilemodel.ImportLifecycleAction{{
			Name: created.Name, Create: true, After: importLifecycleProfile(created),
			Files: importLifecycleFiles(profilemodel.ExportedProfile{
				Provider: profilemodel.ProviderSnapshot{Kind: "openai"}, AuthJSON: "created-auth",
			}),
		}},
	}
	if err := profiles.WriteBundleImportJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := profiles.ImportBundleProfile(ctx, created, map[string][]byte{"auth.json": []byte("created-auth")}, id); err != nil {
		t.Fatal(err)
	}
	if _, err := profiles.SetActive(ctx, created.Name); err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog(profiles, &fakeAccounts{}, "")
	current, active, err := catalog.ActiveStandalone(ctx)
	if err != nil || !active || current.Name != work.Name {
		t.Fatalf("active standalone = %+v, %v, err=%v", current, active, err)
	}
	if _, err := profiles.Resolve(ctx, created.Name); err == nil {
		t.Fatal("interrupted imported profile remains")
	}
	if _, err := os.Lstat(created.CodexHome); !os.IsNotExist(err) {
		t.Fatalf("interrupted profile home remains: %v", err)
	}
	if journals, err := profiles.BundleImportJournals(); err != nil || len(journals) != 0 {
		t.Fatalf("remaining journals = %+v, err=%v", journals, err)
	}
}

func TestCatalogCurrentCodexHomeUsesActiveProfileAndFallback(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	defaultHome := filepath.Join(t.TempDir(), "default-codex")
	catalog := NewCatalog(repo, &fakeAccounts{}, defaultHome)
	if home, err := catalog.CurrentCodexHome(context.Background()); err != nil || home != defaultHome {
		t.Fatalf("fallback home = %q, err=%v", home, err)
	}
	added, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if home, err := catalog.CurrentCodexHome(context.Background()); err != nil || home != added.Profile.CodexHome {
		t.Fatalf("active home = %q, err=%v", home, err)
	}
	if err := repo.ClearActive(context.Background()); err != nil {
		t.Fatal(err)
	}
	if home, err := catalog.CurrentCodexHome(context.Background()); err != nil || home != defaultHome {
		t.Fatalf("cleared active fallback = %q, err=%v", home, err)
	}
}

func TestCatalogCurrentCodexHomePropagatesRecoveryErrors(t *testing.T) {
	wantErr := errors.New("journal read failed")
	repo := &journalErrorRepository{repository: profilerepo.NewStore(t.TempDir()), err: wantErr}
	catalog := NewCatalog(repo, &fakeAccounts{}, "configured-default")
	if home, err := catalog.CurrentCodexHome(context.Background()); home != "" || !errors.Is(err, wantErr) {
		t.Fatalf("home/error = %q/%v", home, err)
	}
}

func addLifecycleTestAccount(t *testing.T, store *accountrepo.FileStore, name, auth string) accountentity.Account {
	t.Helper()
	candidate, err := accountentity.NewAccount(
		accountentity.Identity{Email: name + "@example.test", ChatGPTAccountID: "synthetic-" + name}, name, time.Unix(1, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := store.CreateStagedHome()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte(auth), 0o600); err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitLogin(context.Background(), candidate, staged, false)
	if err != nil {
		t.Fatal(err)
	}
	return committed
}

func assertProfileAuth(t *testing.T, store *profilerepo.Store, home, want string) {
	t.Helper()
	content, err := store.ReadAuthJSON(home)
	if err != nil || string(content) != want {
		t.Fatalf("profile auth = %q, err=%v", content, err)
	}
}

func assertAccountAuth(t *testing.T, store *accountrepo.FileStore, id, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(store.CodexHome(id), "auth.json"))
	if err != nil || string(content) != want {
		t.Fatalf("account auth = %q, err=%v", content, err)
	}
}

type lifecycleOrderRepository struct {
	*profilerepo.Store
	order *[]string
}

func (repo *lifecycleOrderRepository) RemoveBundleImportedProfile(ctx context.Context, action profilemodel.ImportLifecycleAction, id string) error {
	*repo.order = append(*repo.order, "remove:"+action.Name)
	return repo.Store.RemoveBundleImportedProfile(ctx, action, id)
}

func (repo *lifecycleOrderRepository) RestoreBundleImportRollback(
	ctx context.Context,
	name, id string,
	before profilemodel.ImportLifecycleProfile,
) error {
	*repo.order = append(*repo.order, "profile:"+name)
	return repo.Store.RestoreBundleImportRollback(ctx, name, id, before)
}

type lifecycleOrderAccounts struct {
	*accountrepo.FileStore
	order *[]string
}

func (store *lifecycleOrderAccounts) RestoreImportedAuthRollback(ctx context.Context, accountID, id string) error {
	*store.order = append(*store.order, "account:"+accountID)
	return store.FileStore.RestoreImportedAuthRollback(ctx, accountID, id)
}

type journalErrorRepository struct {
	repository
	err error
}

type commitMarkerFailureRepository struct {
	*profilerepo.Store
	persist bool
	failed  bool
}

func (repo *commitMarkerFailureRepository) WriteBundleImportJournal(journal profilemodel.ImportLifecycleJournal) error {
	if journal.Phase == "committed" && !repo.failed {
		repo.failed = true
		if repo.persist {
			if err := repo.Store.WriteBundleImportJournal(journal); err != nil {
				return err
			}
		}
		return errors.New("injected journal sync failure")
	}
	return repo.Store.WriteBundleImportJournal(journal)
}

func (repo *journalErrorRepository) BundleImportJournals() ([]profilemodel.ImportLifecycleJournal, error) {
	return nil, repo.err
}
