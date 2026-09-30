package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
)

type fakeCurrentCodex struct {
	source string
}

func (fake *fakeCurrentCodex) ImportCurrent(_ context.Context, source, staged string) (accountentity.Identity, error) {
	fake.source = source
	if err := os.WriteFile(filepath.Join(staged, "auth.json"), []byte(`{"synthetic":true}`), 0o600); err != nil {
		return accountentity.Identity{}, err
	}
	return accountentity.Identity{Email: "imported@example.com", ChatGPTAccountID: "imported-account"}, nil
}

func TestImportCurrentCommitsManagedAccount(t *testing.T) {
	store := accountrepo.NewFileStore(t.TempDir())
	codex := &fakeCurrentCodex{}
	importer := NewImportCurrent(store, codex, "/synthetic/current-codex")
	importer.now = func() time.Time { return time.Unix(11, 0) }

	account, err := importer.Run(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if codex.source != "/synthetic/current-codex" || account.Name != "main" || account.Email != "imported@example.com" {
		t.Fatalf("import result = source %q account %+v", codex.source, account)
	}
	current, err := store.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != account.ID {
		t.Fatalf("current account = %+v", current)
	}
}
