package account

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateStagedProfileRejectsMissingFileAndSymlink(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := validateStagedProfile(missing); err == nil {
		t.Fatal("missing staged profile unexpectedly accepted")
	}
	file := filepath.Join(t.TempDir(), "profile")
	if err := os.WriteFile(file, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateStagedProfile(file); err == nil {
		t.Fatal("staged file unexpectedly accepted")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := validateStagedProfile(link); err == nil {
		t.Fatal("staged symlink unexpectedly accepted")
	}
}

func TestSecureCodexHomeRejectsNestedSymlink(t *testing.T) {
	home := t.TempDir()
	link := filepath.Join(home, "escape")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if err := secureCodexHome(home); err == nil {
		t.Fatal("profile with nested symlink unexpectedly accepted")
	}
}

func TestTransactionPathSkipsExistingCandidate(t *testing.T) {
	store := newTestStore(t)
	store.now = func() time.Time { return time.Unix(10, 0) }
	base := filepath.Join(store.accountsDir(), "synthetic")
	first := base + ".backup-10000000000-0"
	if err := os.WriteFile(first, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := store.transactionPath(base, "backup")
	if err != nil {
		t.Fatal(err)
	}
	if path == first || path != base+".backup-10000000000-1" {
		t.Fatalf("transaction path = %q", path)
	}
}
