package profile

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
)

func TestOpenAICompatibleLocalConfigRoundTripAndClear(t *testing.T) {
	store := NewStore(t.TempDir())
	home := store.ManagedHome("api-key")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	baseURL := "https://example.test/v1/"
	if err := writeOpenAICompatibleBaseURL(home, &baseURL); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.ReadOpenAICompatibleBaseURL(home)
	if err != nil || !found || got != baseURL {
		t.Fatalf("base URL = %q, found=%t, err=%v", got, found, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(home, profileLocalConfigFileName))
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("local config mode = %v, err=%v", info.Mode().Perm(), err)
		}
	}
	if err := writeOpenAICompatibleBaseURL(home, nil); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ReadOpenAICompatibleBaseURL(home); err != nil || found {
		t.Fatalf("cleared config found=%t err=%v", found, err)
	}
}

func TestOpenAICompatibleLocalConfigReplacesSymlinkWithoutTouchingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires elevated privileges on some Windows runners")
	}
	store := NewStore(t.TempDir())
	home := store.ManagedHome("api-key")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.toml")
	if err := os.WriteFile(target, []byte("do_not_touch = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(home, profileLocalConfigFileName)
	if err := os.Symlink(target, configPath); err != nil {
		t.Fatal(err)
	}
	baseURL := "http://127.0.0.1:11434/v1"
	if err := writeOpenAICompatibleBaseURL(home, &baseURL); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(target)
	if string(content) != "do_not_touch = true\n" {
		t.Fatalf("symlink target changed: %q", content)
	}
	info, err := os.Lstat(configPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("local config remained symlink: mode=%v err=%v", info.Mode(), err)
	}
}

func TestLoginOpenAIAPIKeyCreatesUpdatesPreservesAndClearsBaseURL(t *testing.T) {
	store := NewStore(t.TempDir())
	profile := profileentity.Profile{
		Name: "api_key_example.test", CodexHome: store.ManagedHome("api_key_example.test"), Managed: true,
		Provider: profileentity.Provider{Kind: profileentity.ProviderOpenAI},
	}
	firstURL := "https://example.test/v1"
	created, wasCreated, err := store.LoginOpenAIAPIKey(context.Background(), profile, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"first"}`), &firstURL, true, true)
	if err != nil || !wasCreated || created.Name != profile.Name {
		t.Fatalf("create = %#v, created=%t, err=%v", created, wasCreated, err)
	}
	if got, found, err := store.ReadOpenAICompatibleBaseURL(profile.CodexHome); err != nil || !found || got != firstURL {
		t.Fatalf("created base URL = %q found=%t err=%v", got, found, err)
	}

	updatedProfile := profile
	updatedProfile.Email = ""
	if _, wasCreated, err := store.LoginOpenAIAPIKey(context.Background(), updatedProfile, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"second"}`), nil, false, true); err != nil || wasCreated {
		t.Fatalf("preserve update created=%t err=%v", wasCreated, err)
	}
	if got, found, err := store.ReadOpenAICompatibleBaseURL(profile.CodexHome); err != nil || !found || got != firstURL {
		t.Fatalf("preserved base URL = %q found=%t err=%v", got, found, err)
	}
	auth, err := store.ReadAuthJSON(profile.CodexHome)
	if err != nil || !strings.Contains(string(auth), "second") {
		t.Fatalf("updated auth = %q err=%v", auth, err)
	}
	clear(auth)

	if _, _, err := store.LoginOpenAIAPIKey(context.Background(), updatedProfile, []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"third"}`), nil, true, true); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ReadOpenAICompatibleBaseURL(profile.CodexHome); err != nil || found {
		t.Fatalf("cleared base URL found=%t err=%v", found, err)
	}
}

func TestOpenAICompatibleBaseURLValidationDoesNotEchoSecrets(t *testing.T) {
	for _, value := range []string{
		"https://user:login-password-secret-sentinel@example.test/v1",
		"https://example.test/v1?token=login-query-secret-sentinel",
		"https://example.test/v1#login-fragment-secret-sentinel",
		" not-a-url-login-parse-secret-sentinel ",
	} {
		_, err := validateOpenAICompatibleBaseURL(value)
		if err == nil || !strings.Contains(err.Error(), "credential-free") || strings.Contains(err.Error(), "secret-sentinel") {
			t.Fatalf("validation error = %v", err)
		}
	}
}
