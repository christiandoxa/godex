package profile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func TestBundleImportJournalRecoveryDataKeepsAuthOutOfMetadata(t *testing.T) {
	for _, phase := range []string{"preparing", "applying"} {
		t.Run(phase, func(t *testing.T) {
			store, profile := profileWithAuth(t, []byte(`{"access_token":"previous"}`))
			const id = "0123456789abcdef0123456789abcdef"
			before := importLifecycleSnapshot(profile)
			if err := store.PrepareBundleImportRollback(t.Context(), profile.Name, id, []string{profileAuthFileName}); err != nil {
				t.Fatal(err)
			}
			journal := profilemodel.ImportLifecycleJournal{
				Version: bundleImportVersion, ID: id, Phase: phase,
				Actions: []profilemodel.ImportLifecycleAction{{
					Name: profile.Name, Before: &before, After: before, BackupID: id,
					Files: []profilemodel.ImportLifecycleFile{{Path: profileAuthFileName, SHA256: digestImportTest(`{"access_token":"next"}`)}},
				}},
			}
			if err := store.WriteBundleImportJournal(journal); err != nil {
				t.Fatal(err)
			}
			journalBytes, err := os.ReadFile(store.bundleImportJournalPath(id))
			if err != nil || strings.Contains(string(journalBytes), "access_token") || strings.Contains(string(journalBytes), "previous") || strings.Contains(string(journalBytes), "next") {
				t.Fatalf("lifecycle metadata contains credential data: %q, err=%v", journalBytes, err)
			}
			journals, err := store.BundleImportJournals()
			if err != nil || len(journals) != 1 || journals[0].Phase != phase {
				t.Fatalf("journals = %+v, err=%v", journals, err)
			}
			if phase == "preparing" {
				if err := store.CleanupBundleImportRollback(t.Context(), profile.Name, id); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := store.ReplaceAuth(t.Context(), profile.Name, []byte(`{"access_token":"next"}`)); err != nil {
					t.Fatal(err)
				}
				if err := store.RestoreBundleImportRollback(t.Context(), profile.Name, id, before); err != nil {
					t.Fatal(err)
				}
				assertProfileAuth(t, profile.CodexHome, `{"access_token":"previous"}`)
				if err := store.CleanupBundleImportRollback(t.Context(), profile.Name, id); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.RemoveBundleImportJournal(id); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCleanupOrphanedBundleImportStagingHomes(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".import-crashed", ".provider-import-crashed", "retained"} {
		if err := os.Mkdir(filepath.Join(store.profilesRoot(), name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CleanupOrphanedImportStagingHomes(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".import-crashed", ".provider-import-crashed"} {
		if _, err := os.Lstat(filepath.Join(store.profilesRoot(), name)); !os.IsNotExist(err) {
			t.Fatalf("orphan %s remains: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(store.profilesRoot(), "retained")); err != nil {
		t.Fatalf("unrelated home was removed: %v", err)
	}
}

func TestRemoveBundleImportHomeRequiresMatchingJournalFiles(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	orphan := store.ManagedHome("work")
	if err := os.Mkdir(orphan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "auth.json"), []byte("credential"), 0o600); err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef0123456789abcdef"
	if err := os.WriteFile(filepath.Join(orphan, bundleImportOwnerName(id)), []byte(id), 0o600); err != nil {
		t.Fatal(err)
	}
	action := profilemodel.ImportLifecycleAction{
		Name: "work", Create: true,
		After: profilemodel.ImportLifecycleProfile{CodexHome: orphan, Managed: true, Provider: profilemodel.ProviderSnapshot{Kind: "openai"}},
		Files: []profilemodel.ImportLifecycleFile{{Path: "auth.json", SHA256: digestImportTest("credential")}},
	}
	if err := store.RemoveBundleImportedProfile(context.Background(), action, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Fatalf("unregistered imported profile home remains: %v", err)
	}
}

func TestRemoveBundleImportHomePreservesConflictingAndMetadataOnlyOrphans(t *testing.T) {
	for _, test := range []struct {
		name  string
		file  string
		want  string
		extra bool
		owned bool
	}{
		{name: "different secret", file: "auth.json", want: "other", owned: true},
		{name: "metadata only", want: ""},
		{name: "unexpected extra file", file: "auth.json", want: "expected", extra: true, owned: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			if err := store.Prepare(); err != nil {
				t.Fatal(err)
			}
			orphan := store.ManagedHome("work")
			if err := os.Mkdir(orphan, 0o700); err != nil {
				t.Fatal(err)
			}
			const id = "0123456789abcdef0123456789abcdef"
			if test.owned {
				if err := os.WriteFile(filepath.Join(orphan, bundleImportOwnerName(id)), []byte(id), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var files []profilemodel.ImportLifecycleFile
			if test.file != "" {
				if err := os.WriteFile(filepath.Join(orphan, test.file), []byte(test.want), 0o600); err != nil {
					t.Fatal(err)
				}
				if test.extra {
					if err := os.WriteFile(filepath.Join(orphan, "notes"), []byte("keep"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				files = []profilemodel.ImportLifecycleFile{{Path: test.file, SHA256: digestImportTest("expected")}}
			}
			action := profilemodel.ImportLifecycleAction{
				Name: "work", Create: true,
				After: profilemodel.ImportLifecycleProfile{CodexHome: orphan, Managed: true, Provider: profilemodel.ProviderSnapshot{Kind: "openai"}},
				Files: files,
			}
			if err := store.RemoveBundleImportedProfile(context.Background(), action, id); err == nil {
				t.Fatal("unverified home was removed")
			}
			if _, err := os.Lstat(orphan); err != nil {
				t.Fatalf("unverified home was removed: %v", err)
			}
		})
	}
}

func TestRemoveBundleImportHomeCleansRegisteredMetadataOnlyProfile(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	const id = "abcdef0123456789abcdef0123456789"
	profile := profileentity.Profile{
		Name: "copilot", CodexHome: store.ManagedHome("copilot"), Managed: true,
		Email: "user@example.test", Provider: profileentity.Provider{Kind: profileentity.ProviderCopilot},
	}
	if err := store.ImportBundleProfile(ctx, profile, map[string][]byte{}, id); err != nil {
		t.Fatal(err)
	}
	action := profilemodel.ImportLifecycleAction{Name: profile.Name, Create: true, After: importLifecycleSnapshot(profile)}
	if err := store.RemoveBundleImportedProfile(ctx, action, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(profile.CodexHome); !os.IsNotExist(err) {
		t.Fatalf("metadata-only profile home remains: %v", err)
	}
}

func TestRemoveBundleImportHomeRequiresMarkerForRegisteredProfile(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	const id = "abcdef0123456789abcdef0123456789"
	profile := profileentity.Profile{
		Name: "work", CodexHome: store.ManagedHome("work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := store.ImportOpenAI(ctx, profile, []byte("credential"), false); err != nil {
		t.Fatal(err)
	}
	action := profilemodel.ImportLifecycleAction{
		Name: profile.Name, Create: true, After: importLifecycleSnapshot(profile),
		Files: []profilemodel.ImportLifecycleFile{{Path: "auth.json", SHA256: digestImportTest("credential")}},
	}
	if err := store.RemoveBundleImportedProfile(ctx, action, id); err == nil {
		t.Fatal("registered profile without the import marker was removed")
	}
	if _, err := store.Resolve(ctx, profile.Name); err != nil {
		t.Fatalf("registered profile was removed: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(profile.CodexHome, "auth.json")); err != nil || string(got) != "credential" {
		t.Fatalf("profile auth = %q, err=%v", got, err)
	}
}

func TestRemoveBundleImportHomeReplaysRemovalCrashPhases(t *testing.T) {
	for _, phase := range []string{"home", "quarantined", "partially deleted"} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			store := NewStore(t.TempDir())
			const id = "0123456789abcdef0123456789abcdef"
			profile := profileentity.Profile{
				Name: "work", CodexHome: store.ManagedHome("work"), Managed: true,
				Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
			}
			if err := store.ImportBundleProfile(ctx, profile, map[string][]byte{"auth.json": []byte("credential")}, id); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Remove(ctx, profile.Name, false); err != nil {
				t.Fatal(err)
			}
			removal := store.bundleImportRemovalPath(profile.Name, id)
			if phase != "home" {
				if err := os.Rename(profile.CodexHome, removal); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "partially deleted" {
				if err := os.Remove(filepath.Join(removal, bundleImportOwnerName(id))); err != nil {
					t.Fatal(err)
				}
			}
			action := profilemodel.ImportLifecycleAction{
				Name: profile.Name, Create: true, After: importLifecycleSnapshot(profile),
				Files: []profilemodel.ImportLifecycleFile{{Path: "auth.json", SHA256: digestImportTest("credential")}},
			}
			if err := store.RemoveBundleImportedProfile(ctx, action, id); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{profile.CodexHome, removal} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("removal path %q remains: %v", path, err)
				}
			}
		})
	}
}

func TestRemoveBundleImportHomeRejectsQuarantineSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not portable on Windows")
	}
	store := NewStore(t.TempDir())
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef0123456789abcdef"
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "credential")
	if err := os.WriteFile(secret, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	removal := store.bundleImportRemovalPath("work", id)
	if err := os.Symlink(outside, removal); err != nil {
		t.Fatal(err)
	}
	action := profilemodel.ImportLifecycleAction{
		Name: "work", Create: true,
		After: profilemodel.ImportLifecycleProfile{CodexHome: store.ManagedHome("work"), Managed: true, Provider: profilemodel.ProviderSnapshot{Kind: "openai"}},
	}
	if err := store.RemoveBundleImportedProfile(context.Background(), action, id); err == nil {
		t.Fatal("quarantine symlink was accepted")
	}
	if got, err := os.ReadFile(secret); err != nil || string(got) != "keep" {
		t.Fatalf("outside file = %q, err=%v", got, err)
	}
}

func TestRemoveBundleImportHomePreservesRegisteredProfileWithDifferentMetadata(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	const id = "0123456789abcdef0123456789abcdef"
	profile := profileentity.Profile{
		Name: "work", CodexHome: store.ManagedHome("work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := store.ImportOpenAI(ctx, profile, []byte("credential"), false); err != nil {
		t.Fatal(err)
	}
	other := importLifecycleSnapshot(profile)
	other.Provider.Kind = "anthropic"
	action := profilemodel.ImportLifecycleAction{
		Name: profile.Name, Create: true, After: other,
		Files: []profilemodel.ImportLifecycleFile{{Path: "auth.json", SHA256: digestImportTest("credential")}},
	}
	if err := store.RemoveBundleImportedProfile(ctx, action, id); err == nil {
		t.Fatal("profile with different metadata was removed")
	}
	if _, err := store.Resolve(ctx, profile.Name); err != nil {
		t.Fatalf("conflicting profile was removed: %v", err)
	}
}

func TestRemoveBundleImportHomeCleansOwnedMetadataOnlyOrphan(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef0123456789abcdef"
	home := store.ManagedHome("copilot")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, bundleImportOwnerName(id)), []byte(id), 0o600); err != nil {
		t.Fatal(err)
	}
	action := profilemodel.ImportLifecycleAction{
		Name: "copilot", Create: true,
		After: profilemodel.ImportLifecycleProfile{CodexHome: home, Managed: true, Provider: profilemodel.ProviderSnapshot{Kind: "copilot"}},
	}
	if err := store.RemoveBundleImportedProfile(context.Background(), action, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(home); !os.IsNotExist(err) {
		t.Fatalf("owned metadata-only orphan remains: %v", err)
	}
}

func TestRemoveBundleImportHomePreservesForeignOwnerMarker(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Prepare(); err != nil {
		t.Fatal(err)
	}
	const id = "0123456789abcdef0123456789abcdef"
	home := store.ManagedHome("copilot")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, bundleImportOwnerName(id)), []byte("abcdef0123456789abcdef0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	action := profilemodel.ImportLifecycleAction{
		Name: "copilot", Create: true,
		After: profilemodel.ImportLifecycleProfile{CodexHome: home, Managed: true, Provider: profilemodel.ProviderSnapshot{Kind: "copilot"}},
	}
	if err := store.RemoveBundleImportedProfile(context.Background(), action, id); err == nil {
		t.Fatal("home with a foreign lifecycle marker was removed")
	}
	if _, err := os.Lstat(home); err != nil {
		t.Fatalf("home with a foreign lifecycle marker was removed: %v", err)
	}
}

func TestCleanupBundleImportOwnerMarker(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	const id = "abcdef0123456789abcdef0123456789"
	profile := profileentity.Profile{Name: "copilot", CodexHome: store.ManagedHome("copilot"), Managed: true, Provider: profileentity.Provider{Kind: profileentity.ProviderCopilot}}
	if err := store.ImportBundleProfile(ctx, profile, map[string][]byte{}, id); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanupBundleImportOwnerMarker(ctx, profile.Name, id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(profile.CodexHome, bundleImportOwnerName(id))); !os.IsNotExist(err) {
		t.Fatalf("owner marker remains after commit cleanup: %v", err)
	}
}

func importLifecycleSnapshot(profile profileentity.Profile) profilemodel.ImportLifecycleProfile {
	return profilemodel.ImportLifecycleProfile{
		CodexHome: profile.CodexHome, Managed: profile.Managed, Email: profile.Email,
		Provider: profilemodel.ProviderSnapshot{Kind: string(profile.Provider.Kind)},
	}
}

func digestImportTest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func TestBundleImportJournalModeIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not portable on Windows")
	}
	store, profile := profileWithAuth(t, []byte(`{"access_token":"synthetic"}`))
	const id = "abcdef0123456789abcdef0123456789"
	before := importLifecycleSnapshot(profile)
	if err := store.PrepareBundleImportRollback(t.Context(), profile.Name, id, []string{profileAuthFileName}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteBundleImportJournal(profilemodel.ImportLifecycleJournal{
		Version: bundleImportVersion, ID: id, Phase: "preparing",
		Actions: []profilemodel.ImportLifecycleAction{{Name: profile.Name, Before: &before, After: before, BackupID: id}},
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(store.bundleImportJournalPath(id))
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("journal mode = %v, err=%v", info, err)
	}
}
