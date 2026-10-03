package profile

import (
	"context"
	"path/filepath"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type quotaCatalogInspector struct{}

func (quotaCatalogInspector) InspectAuthJSON(context.Context, []byte) (accountentity.Identity, error) {
	return accountentity.Identity{}, nil
}

func (quotaCatalogInspector) InspectQuotaAuth(_ context.Context, home string) (profilemodel.QuotaAuthSummary, error) {
	if filepath.Base(home) == "codex" {
		return profilemodel.QuotaAuthSummary{Label: "chatgpt", Compatible: true}, nil
	}
	return profilemodel.QuotaAuthSummary{Label: "no-auth"}, nil
}

func TestCatalogQuotaTargetsIncludeAccountsAndStandaloneProfiles(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	accounts := &fakeAccounts{values: []accountentity.Account{{ID: "legacy-id", Name: "legacy", Enabled: false}}, current: "legacy-id"}
	catalog := NewCatalog(repo, accounts, t.TempDir())
	catalog.SetAuthInspector(quotaCatalogInspector{})
	if _, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "standalone", Activate: true}); err != nil {
		t.Fatal(err)
	}
	targets, err := catalog.QuotaTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %+v", targets)
	}
	byName := map[string]profilemodel.QuotaTarget{}
	for _, target := range targets {
		byName[target.Name] = target
	}
	if byName["legacy"].Enabled || byName["legacy"].Auth != "chatgpt" || !byName["legacy"].Compatible {
		t.Fatalf("legacy target = %+v", byName["legacy"])
	}
	if !byName["standalone"].Enabled || !byName["standalone"].Active || byName["standalone"].Auth != "no-auth" {
		t.Fatalf("standalone target = %+v", byName["standalone"])
	}
}

type modelProviderQuotaCatalogInspector struct{}

func (modelProviderQuotaCatalogInspector) InspectAuthJSON(context.Context, []byte) (accountentity.Identity, error) {
	return accountentity.Identity{}, nil
}

func (modelProviderQuotaCatalogInspector) InspectQuotaAuth(context.Context, string) (profilemodel.QuotaAuthSummary, error) {
	return profilemodel.QuotaAuthSummary{Label: "model-provider:amazon-bedrock", Compatible: false}, nil
}

func TestCatalogQuotaTargetsPreserveModelProviderAuthSummary(t *testing.T) {
	repo := profilerepo.NewStore(t.TempDir())
	catalog := NewCatalog(repo, &fakeAccounts{}, t.TempDir())
	catalog.SetAuthInspector(modelProviderQuotaCatalogInspector{})
	if _, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "bedrock", Activate: true}); err != nil {
		t.Fatal(err)
	}
	targets, err := catalog.QuotaTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Auth != "model-provider:amazon-bedrock" || targets[0].Compatible {
		t.Fatalf("quota targets = %#v", targets)
	}
}
