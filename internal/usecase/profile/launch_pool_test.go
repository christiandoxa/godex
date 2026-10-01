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
		{"claude-b", profileentity.ProviderAnthropic, false},
		{"claude-a", profileentity.ProviderAnthropic, false},
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
	claudePool, err := catalog.ProviderLaunchPool(context.Background(), "claude-a", "anthropic", true)
	if err != nil || len(claudePool) != 2 || claudePool[0].Name != "claude-a" || claudePool[1].Name != "claude-b" {
		t.Fatalf("Anthropic rotating pool = %#v, err = %v", claudePool, err)
	}
	claudeExplicit, err := catalog.ProviderLaunchPool(context.Background(), "claude-a", "anthropic", false)
	if err != nil || len(claudeExplicit) != 1 || claudeExplicit[0].Name != "claude-a" {
		t.Fatalf("Anthropic explicit pool = %#v, err = %v", claudeExplicit, err)
	}
}

func TestResolveProviderLaunchPrefersProviderThenSafeProfileFallback(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	for _, fixture := range []struct {
		name     string
		provider profileentity.ProviderKind
		active   bool
	}{
		{"openai-home", profileentity.ProviderOpenAI, true},
		{"claude-b", profileentity.ProviderAnthropic, false},
		{"claude-a", profileentity.ProviderAnthropic, false},
	} {
		profile := profileentity.Profile{
			Name: fixture.name, CodexHome: repo.ManagedHome(fixture.name), Managed: true,
			Provider: profileentity.Provider{Kind: fixture.provider},
		}
		if err := repo.ImportProvider(context.Background(), profile, map[string]string{}, fixture.active); err != nil {
			t.Fatal(err)
		}
	}

	selected, found, err := catalog.ResolveProviderLaunch(context.Background(), "anthropic", "")
	if err != nil || !found || selected.Name != "claude-a" || selected.Provider != "anthropic" {
		t.Fatalf("Anthropic selection = %#v, found=%t, err=%v", selected, found, err)
	}
	explicit, found, err := catalog.ResolveProviderLaunch(context.Background(), "anthropic", "openai-home")
	if err != nil || !found || explicit.Name != "openai-home" || explicit.Provider != "openai" {
		t.Fatalf("explicit home selection = %#v, found=%t, err=%v", explicit, found, err)
	}
	fallback, found, err := catalog.ResolveProviderLaunch(context.Background(), "kiro", "")
	if err != nil || !found || fallback.Name != "openai-home" {
		t.Fatalf("generic fallback = %#v, found=%t, err=%v", fallback, found, err)
	}
}
