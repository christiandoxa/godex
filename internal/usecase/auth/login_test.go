package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	accountmodel "github.com/christiandoxa/godex/internal/model/account"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
)

type fakeCodex struct {
	identities []accountentity.Identity
}

func (fake *fakeCodex) Login(_ context.Context, home string, _ bool) (accountentity.Identity, error) {
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"synthetic":true}`), 0o600); err != nil {
		return accountentity.Identity{}, err
	}
	identity := fake.identities[0]
	fake.identities = fake.identities[1:]
	return identity, nil
}

func TestLoginDeduplicatesAndPreservesName(t *testing.T) {
	store := accountrepo.NewFileStore(t.TempDir())
	codex := &fakeCodex{identities: []accountentity.Identity{
		{Email: "first@example.com", ChatGPTAccountID: "account-1"},
		{Email: "changed@example.com", ChatGPTAccountID: "account-1"},
	}}
	login := NewLogin(store, codex)
	login.now = func() time.Time { return time.Unix(1, 0) }

	first, err := login.Run(context.Background(), accountmodel.LoginInput{Name: "work"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := login.Run(context.Background(), accountmodel.LoginInput{})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.Name != "work" {
		t.Fatalf("second login = %#v", second)
	}
	accounts, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("account count = %d", len(accounts))
	}
}

func TestLoginReportsStagedHomeCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("synthetic cleanup failure")
	store := accountrepo.NewFileStore(t.TempDir())
	login := NewLogin(store, &fakeCodex{identities: []accountentity.Identity{
		{Email: "cleanup@example.com", ChatGPTAccountID: "cleanup-account"},
	}})
	login.removeAll = func(string) error { return cleanupErr }

	_, err := login.Run(context.Background(), accountmodel.LoginInput{})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error = %v", err)
	}
}
