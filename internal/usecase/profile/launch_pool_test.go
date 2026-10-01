package profile

import (
	"context"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

func TestProviderLaunchPoolKeepsSelectedFirstAndFiltersProvider(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	for _, fixture := range []struct {
		name     string
		provider profileentity.ProviderKind
		active   bool
	}{
		{"copilot-b", profileentity.ProviderCopilot, false},
		{"copilot-a", profileentity.ProviderCopilot, true},
		{"claude", profileentity.ProviderAnthropic, false},
	} {
		profile := profileentity.Profile{
			Name: fixture.name, CodexHome: repo.ManagedHome(fixture.name), Managed: true,
			Provider: profileentity.Provider{Kind: fixture.provider},
		}
		if err := repo.ImportProvider(context.Background(), profile, map[string]string{}, fixture.active); err != nil {
			t.Fatal(err)
		}
	}

	pool, err := catalog.ProviderLaunchPool(context.Background(), "copilot-a", "copilot", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 2 || pool[0].Name != "copilot-a" || pool[1].Name != "copilot-b" {
		t.Fatalf("rotating pool = %#v", pool)
	}
	explicit, err := catalog.ProviderLaunchPool(context.Background(), "copilot-a", "copilot", false)
	if err != nil || len(explicit) != 1 || explicit[0].Name != "copilot-a" {
		t.Fatalf("explicit pool = %#v, err = %v", explicit, err)
	}
	if _, err := catalog.ProviderLaunchPool(context.Background(), "copilot-a", "anthropic", true); err == nil {
		t.Fatal("provider mismatch unexpectedly accepted")
	}
}
