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
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type selectedLoginInspector struct{}

func (selectedLoginInspector) InspectAuthJSON(_ context.Context, content []byte) (accountentity.Identity, error) {
	if !bytes.Contains(content, []byte("selected-account")) {
		return accountentity.Identity{}, errors.New("invalid selected auth fixture")
	}
	return accountentity.Identity{Email: "selected@example.com", ChatGPTAccountID: "selected-account"}, nil
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
