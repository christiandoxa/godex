package profile

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type selectedLoginInspector struct{}

func (selectedLoginInspector) InspectAuthJSON(_ context.Context, content []byte) (accountentity.Identity, error) {
	if !bytes.Contains(content, []byte("selected-account")) {
		return accountentity.Identity{}, errors.New("invalid selected auth fixture")
	}
	return accountentity.Identity{Email: "selected@example.com", ChatGPTAccountID: "selected-account"}, nil
}

func (selectedLoginInspector) InspectQuotaAuth(_ context.Context, home string) (profilemodel.QuotaAuthSummary, error) {
	content, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if errors.Is(err, os.ErrNotExist) {
		return profilemodel.QuotaAuthSummary{Label: "no-auth"}, nil
	}
	if err != nil {
		return profilemodel.QuotaAuthSummary{}, err
	}
	if bytes.Contains(content, []byte("\"auth_mode\":\"apikey\"")) || bytes.Contains(content, []byte("OPENAI_API_KEY")) {
		return profilemodel.QuotaAuthSummary{Label: "api-key"}, nil
	}
	return profilemodel.QuotaAuthSummary{Label: "chatgpt", Compatible: true}, nil
}

func selectedLoginFixture() []byte {
	return []byte("{\"auth_mode\":\"chatgpt\",\"tokens\":{\"access_token\":\"fixture-token\",\"account_id\":\"selected-account\"}}")
}

func TestProdex04356SelectedLoginHoldsLifecycleLockAcrossExternalLogin(t *testing.T) {
	root := t.TempDir()
	repo := profilerepo.NewStore(root)
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(root, "current"))
	if _, err := catalog.Add(t.Context(), profilemodel.AddRequest{Name: "work"}); err != nil {
		t.Fatal(err)
	}
	catalog.SetAuthInspector(selectedLoginInspector{})
	entered := make(chan struct{})
	release := make(chan struct{})
	loginDone := make(chan error, 1)
	go func() {
		_, err := catalog.SelectedOpenAILogin(t.Context(), "work", func() ([]byte, error) {
			close(entered)
			<-release
			return selectedLoginFixture(), nil
		})
		loginDone <- err
	}()
	<-entered

	removeDone := make(chan error, 1)
	go func() {
		_, err := catalog.Remove(t.Context(), profilemodel.RemoveRequest{Name: "work", DeleteHome: true})
		removeDone <- err
	}()
	select {
	case err := <-removeDone:
		t.Fatalf("profile mutation escaped selected-login lifecycle lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-loginDone; err != nil {
		t.Fatal(err)
	}
	if err := <-removeDone; err != nil {
		t.Fatal(err)
	}
}

func TestProdex04356SelectedLoginRejectsTargetRecreatedDuringExternalLogin(t *testing.T) {
	root := t.TempDir()
	repo := profilerepo.NewStore(root)
	catalog := NewCatalog(repo, &fakeAccounts{}, filepath.Join(root, "current"))
	if _, err := catalog.Add(t.Context(), profilemodel.AddRequest{Name: "work"}); err != nil {
		t.Fatal(err)
	}
	catalog.SetAuthInspector(selectedLoginInspector{})
	entered := make(chan struct{})
	release := make(chan struct{})
	loginDone := make(chan error, 1)
	go func() {
		_, err := catalog.SelectedOpenAILogin(t.Context(), "work", func() ([]byte, error) {
			close(entered)
			<-release
			return selectedLoginFixture(), nil
		})
		loginDone <- err
	}()
	<-entered

	if _, err := repo.Remove(t.Context(), "work", true); err != nil {
		t.Fatal(err)
	}
	replacement := profileentity.Profile{
		Name: "work", CodexHome: repo.ManagedHome("work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderGemini},
	}
	if err := repo.ImportProvider(t.Context(), replacement, map[string]string{}, true); err != nil {
		t.Fatal(err)
	}
	close(release)
	err := <-loginDone
	if err == nil || !strings.Contains(err.Error(), "changed while login was running") {
		t.Fatalf("recreated selected-login target error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(replacement.CodexHome, "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary selected credentials reached replacement profile: %v", err)
	}
}

func TestProdex04356SelectedAPIKeyManagedAccountBecomesActiveDirectLaunchTarget(t *testing.T) {
	root := t.TempDir()
	accounts := accountrepo.NewFileStore(root)
	account := addLifecycleTestAccount(t, accounts, "work", "before")
	profiles := profilerepo.NewStore(root)
	catalog := NewCatalog(profiles, accounts, filepath.Join(root, "current"))
	catalog.SetAuthInspector(selectedLoginInspector{})
	const baseURL = "https://api.example.test/v1"
	report, err := catalog.SelectedOpenAIAPIKey(t.Context(), "work", profilemodel.APIKeyLoginInput{
		APIKey: "sk-selected", BaseURL: baseURL, BaseURLSpecified: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.AccountID != account.ID || report.Profile.Email != "" || !report.Active {
		t.Fatalf("selected API-key report = %#v", report)
	}
	target, active, err := catalog.ActiveLaunch(t.Context())
	if err != nil || !active {
		t.Fatalf("active API-key target = %#v active=%t err=%v", target, active, err)
	}
	if target.Name != "work" || target.AccountID != account.ID || target.CodexHome != accounts.CodexHome(account.ID) ||
		target.Provider != "openai" || target.Auth != "api-key" {
		t.Fatalf("active API-key target = %#v", target)
	}
	gotBase, found, err := catalog.OpenAICompatibleBaseURL(t.Context(), "work")
	if err != nil || !found || gotBase != baseURL {
		t.Fatalf("active API-key base URL = %q found=%t err=%v", gotBase, found, err)
	}
	release, err := catalog.AcquireLaunch(t.Context(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if mutationRelease, err := accounts.AcquireProfileMutation(t.Context(), account.ID); err == nil {
		_ = mutationRelease()
		t.Fatal("account mutation escaped active API-key launch lease")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	mutationRelease, err := accounts.AcquireProfileMutation(t.Context(), account.ID)
	if err != nil {
		t.Fatalf("account mutation stayed blocked after launch release: %v", err)
	}
	if err := mutationRelease(); err != nil {
		t.Fatal(err)
	}
}
