package profile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type fakeAccounts struct {
	values  []accountentity.Account
	current string
}

func (fake *fakeAccounts) List(context.Context) ([]accountentity.Account, error) {
	return append([]accountentity.Account(nil), fake.values...), nil
}
func (fake *fakeAccounts) Current(context.Context) (accountentity.Account, error) {
	for _, value := range fake.values {
		if value.ID == fake.current {
			return value, nil
		}
	}
	return accountentity.Account{}, errors.New("no active account")
}
func (fake *fakeAccounts) ActiveID(context.Context) (string, error) { return fake.current, nil }
func (fake *fakeAccounts) ClearActive(context.Context) error {
	fake.current = ""
	return nil
}
func (fake *fakeAccounts) SetActive(_ context.Context, selector string) (accountentity.Account, error) {
	for _, value := range fake.values {
		if value.Name == selector {
			fake.current = value.ID
			return value, nil
		}
	}
	return accountentity.Account{}, errors.New("not found")
}
func (fake *fakeAccounts) RemoveProfile(_ context.Context, selector string, _ bool) (accountentity.Account, error) {
	for index, value := range fake.values {
		if value.ID == selector || value.Name == selector {
			fake.values = append(fake.values[:index], fake.values[index+1:]...)
			return value, nil
		}
	}
	return accountentity.Account{}, errors.New("not found")
}

func (fake *fakeAccounts) ReplaceImportedAuth(_ context.Context, selector string, authJSON []byte) error {
	for _, value := range fake.values {
		if value.ID == selector || value.Name == selector {
			return nil
		}
	}
	return errors.New("not found")
}
func (fake *fakeAccounts) PrepareImportedAuthRollback(context.Context, string, string) error {
	return nil
}
func (fake *fakeAccounts) RestoreImportedAuthRollback(context.Context, string, string) error {
	return nil
}
func (fake *fakeAccounts) CleanupImportedAuthRollback(context.Context, string, string) error {
	return nil
}

func (fake *fakeAccounts) CodexHome(id string) string {
	return filepath.Join(os.TempDir(), "godex-profile-test", id, "codex")
}

func TestCatalogBridgesAccountProfilesAndNewProfiles(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	accounts := &fakeAccounts{values: []accountentity.Account{{ID: "account-id", Name: "legacy", Email: "legacy@example.test", Enabled: true}}, current: "account-id"}
	catalog := NewCatalog(repo, accounts, filepath.Join(t.TempDir(), "current"))
	listed, err := catalog.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].Profile.Name != "legacy" || !listed[0].Active {
		t.Fatalf("legacy list = %+v, err = %v", listed, err)
	}
	added, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "empty", Activate: true})
	if err != nil || added.Profile.Name != "empty" || !added.Active {
		t.Fatalf("added = %+v, err = %v", added, err)
	}
	current, err := catalog.Current(context.Background())
	if err != nil || current.Profile.Name != "empty" {
		t.Fatalf("current = %+v, err = %v", current, err)
	}
	used, err := catalog.Use(context.Background(), "legacy")
	if err != nil || used.Profile.Name != "legacy" || accounts.current != "account-id" {
		t.Fatalf("use legacy = %+v, err = %v", used, err)
	}
}

func TestProdex04356CurrentLaunchResolvesManagedAccountReadOnly(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	accounts := &fakeAccounts{
		values:  []accountentity.Account{{ID: "account-id", Name: "managed", Enabled: true}},
		current: "account-id",
	}
	catalog := NewCatalog(repo, accounts, filepath.Join(t.TempDir(), "current"))
	target, err := catalog.CurrentLaunch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "managed" || target.AccountID != "account-id" ||
		target.CodexHome != accounts.CodexHome("account-id") || target.Provider != "openai" {
		t.Fatalf("current launch = %#v", target)
	}
	if accounts.current != "account-id" {
		t.Fatalf("current launch mutated account selection: %q", accounts.current)
	}
}

func TestCatalogRejectsDuplicateNamesAndHomes(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	accounts := &fakeAccounts{values: []accountentity.Account{{ID: "account-id", Name: "legacy", Enabled: true}}, current: "account-id"}
	catalog := NewCatalog(repo, accounts, accounts.CodexHome("account-id"))
	if _, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "legacy"}); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "external", CodexHome: accounts.CodexHome("account-id"), Insecure: true}); err == nil {
		t.Fatal("duplicate home accepted")
	}
}

func TestSessionProfilesOpenAICompatibleRoutingIncludesEndpoint(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	baseURL := "https://upstream.example/v1"
	profile := profileentity.Profile{
		Name: "api-key", CodexHome: repo.ManagedHome("api-key"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if _, created, err := repo.LoginOpenAIAPIKey(
		context.Background(), profile,
		[]byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"synthetic-key"}`),
		&baseURL, true, true,
	); err != nil || !created {
		t.Fatalf("create API-key profile: created=%t err=%v", created, err)
	}

	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	profiles, err := catalog.SessionProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("session profiles = %+v", profiles)
	}
	want := profilemodel.RoutingID("openai-compatible:" + profile.CodexHome + ":" + baseURL)
	if len(profiles[0].RoutingIDs) != 1 || profiles[0].RoutingIDs[0] != want {
		t.Fatalf("routing IDs = %v, want %q", profiles[0].RoutingIDs, want)
	}
}
