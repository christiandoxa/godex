package profile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
)

type rotationAuthInspector04360 struct{}

func (rotationAuthInspector04360) InspectAuthJSON(context.Context, []byte) (accountentity.Identity, error) {
	return accountentity.Identity{}, nil
}
func (rotationAuthInspector04360) InspectQuotaAuth(_ context.Context, home string) (profilemodel.QuotaAuthSummary, error) {
	if filepath.Base(home) == "ready" {
		return profilemodel.QuotaAuthSummary{Label: "chatgpt", Compatible: true}, nil
	}
	return profilemodel.QuotaAuthSummary{Label: "no-auth", Compatible: false}, nil
}

func TestProdex04360RotationEligibilityUsesActualProfileCountAndAuth(t *testing.T) {
	root := t.TempDir()
	catalog := NewCatalog(profilerepo.NewStore(root), &fakeAccounts{}, t.TempDir())
	catalog.SetAuthInspector(rotationAuthInspector04360{})
	add := func(name string) {
		t.Helper()
		if _, err := catalog.Add(t.Context(), profilemodel.AddRequest{Name: name, Activate: name == "first"}); err != nil {
			t.Fatal(err)
		}
	}
	verify := func(name string, want bool) {
		t.Helper()
		eligible, err := catalog.RuntimeRotationEligible(t.Context(), name)
		if err != nil || eligible != want {
			t.Fatalf("profile=%s rotation eligible=%t want=%t err=%v", name, eligible, want, err)
		}
	}
	add("first")
	verify("first", false) // one profile never enables normal rotation
	add("unready")
	verify("first", false) // two empty credentials are not a pool
	add("ready")
	verify("first", true) // one ready quota-compatible profile makes the pool useful
	verify("missing", false)
	if err := os.RemoveAll(filepath.Join(root, "profiles", "first")); err != nil {
		t.Fatal(err)
	}
	verify("first", false)
}
