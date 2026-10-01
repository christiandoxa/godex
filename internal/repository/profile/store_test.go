package profile

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
)

func TestManagedProfileLifecycle(t *testing.T) {
	store := NewStore(t.TempDir())
	value := profileentity.Profile{
		Name: "work", CodexHome: store.ManagedHome("work"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := store.Create(context.Background(), value, "", false, true); err != nil {
		t.Fatal(err)
	}
	current, err := store.Current(context.Background())
	if err != nil || current != value {
		t.Fatalf("current = %+v, err = %v", current, err)
	}
	if _, err := os.Stat(value.CodexHome); err != nil {
		t.Fatal(err)
	}
	removed, err := store.Remove(context.Background(), value.Name, false)
	if err != nil || removed.Name != value.Name {
		t.Fatalf("removed = %+v, err = %v", removed, err)
	}
	if _, err := os.Stat(value.CodexHome); err != nil {
		t.Fatalf("retained home = %v", err)
	}
}

func TestManagedProfileCopyAndDelete(t *testing.T) {
	store := NewStore(t.TempDir())
	source := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(source, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "config.toml"), []byte("synthetic = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value := profileentity.Profile{
		Name: "copy", CodexHome: store.ManagedHome("copy"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := store.Create(context.Background(), value, source, false, false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(value.CodexHome, "config.toml"))
	if err != nil || string(content) != "synthetic = true\n" {
		t.Fatalf("copied config = %q, err = %v", content, err)
	}
	if _, err := store.Remove(context.Background(), value.Name, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(value.CodexHome); !os.IsNotExist(err) {
		t.Fatalf("deleted home still exists: %v", err)
	}
}

func TestExternalProfileIsRegisteredButNeverDeleted(t *testing.T) {
	store := NewStore(t.TempDir())
	external := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(external, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	value := profileentity.Profile{
		Name: "external", CodexHome: external, Managed: false,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	if err := store.Create(context.Background(), value, "", false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Remove(context.Background(), value.Name, true); err == nil {
		t.Fatal("external home deletion unexpectedly accepted")
	}
	if _, err := os.Stat(external); err != nil {
		t.Fatal(err)
	}
}

func TestProfileCopyRejectsSymlink(t *testing.T) {
	store := NewStore(t.TempDir())
	source := t.TempDir()
	if runtime.GOOS != "windows" {
		_ = os.Chmod(source, 0o700)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(source, "linked")); err != nil {
		t.Skip(err)
	}
	value := profileentity.Profile{Name: "copy", CodexHome: store.ManagedHome("copy"), Managed: true, Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI}}
	if err := store.Create(context.Background(), value, source, true, false); err == nil {
		t.Fatal("symlink source unexpectedly copied")
	}
}
