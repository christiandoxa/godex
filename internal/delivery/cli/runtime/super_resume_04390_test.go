package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type superResumeProfiles04390 struct{ fakeLocalLaunchProfiles }

func (profiles *superResumeProfiles04390) SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error) {
	return []sessionmodel.ProfileHome{{
		Name: profiles.target.Name, CodexHome: profiles.target.CodexHome, Provider: "openai", Enabled: true,
	}}, nil
}

func TestProdex04390SuperSelectedAccountKeepsDurableSessionOwner(t *testing.T) {
	home, owner := t.TempDir(), t.TempDir()
	for _, directory := range []string{home, owner} {
		if err := os.WriteFile(filepath.Join(directory, "config.toml"), []byte("model = \"synthetic-model\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	accounts := superSessionAccounts{homes: map[string]string{"home": home, "owner": owner}}
	profiles := &fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
		Name: "main", AccountID: "home", CodexHome: home, Provider: "openai", Auth: "chatgpt",
	}}
	process := &kiroShortcutProcess{}
	var captured proxymodel.Config
	runner := runtimeusecase.NewRunner(accounts, process, func(config proxymodel.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return &kiroShortcutProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))
	const sessionID = "01900000-0000-7000-8000-000000000165"
	catalog := sessionusecase.NewCatalog(accounts, superSessionReader{home: home, id: sessionID}, nil)
	catalog.SetOwnerLookup(func(context.Context, string) (string, error) { return "owner", nil })
	options, err := parseSuperArguments([]string{"--profile", "main", "--no-sub-agent", "exec", "resume", sessionID, "continue"})
	if err != nil {
		t.Fatal(err)
	}
	if err := launchSuperProfiles(t.Context(), runner, catalog, profiles, options); err != nil {
		t.Fatal(err)
	}
	routed, err := captured.Accounts(t.Context())
	if err != nil || len(routed) != 1 || routed[0].ID != "owner" || routed[0].Home != owner || captured.PreferredAccount != "owner" {
		t.Fatal("selected launch home replaced the durable upstream owner")
	}
}

func TestProdex04390SuperSelectedProfileResumesLegacyRollout(t *testing.T) {
	for _, local := range []bool{false, true} {
		name := "native"
		if local {
			name = "local"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			auth := []byte(`{"OPENAI_API_KEY":"synthetic-profile-key"}`)
			for path, data := range map[string][]byte{"auth.json": auth, "config.toml": []byte("model = \"synthetic-model\"\n")} {
				if err := os.WriteFile(filepath.Join(home, path), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			profiles := &superResumeProfiles04390{fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
				Name: "main", CodexHome: home, Provider: "openai", Auth: "api-key",
			}}}
			process := &localSuperCaptureProcess{}
			var captured proxymodel.Config
			runner := runtimeusecase.NewRunner(nil, process, func(config proxymodel.Config) (runtimeusecase.Proxy, error) {
				captured = config
				return &kiroShortcutProxy{}, nil
			})
			runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))
			const sessionID = "01900000-0000-7000-8000-000000000165"
			catalog := sessionusecase.NewCatalog(&fakeRunnerAccounts{}, superSessionReader{home: home, id: sessionID}, nil)
			catalog.SetSharedCodexHome(t.TempDir())
			catalog.SetProfiles(profiles)
			arguments := []string{"--profile", "main", "--no-presidio", "--no-sub-agent", "--skip-quota-check", "--no-auto-rotate"}
			if local {
				arguments = append(arguments, "--url", "http://127.0.0.1:11434", "--model", "synthetic-local")
			}
			arguments = append(arguments, "exec", "resume", sessionID, "continue")
			var output bytes.Buffer
			if err := superProfilesWithToolLookup(t.Context(), runner, catalog, profiles, &output, arguments,
				func(string) (string, bool) { return "", false }); err != nil {
				t.Fatal(err)
			}
			if process.home == "" || process.home == home || !captured.SmartContextEnabled {
				t.Fatalf("Super overlay/home/context = %q / %t", process.home, captured.SmartContextEnabled)
			}
			accounts, err := captured.Accounts(t.Context())
			if err != nil || len(accounts) != 1 || accounts[0].Home != home {
				t.Fatalf("selected profile routing lost: accounts=%d error=%v", len(accounts), err)
			}
			for _, want := range []string{"--dangerously-bypass-approvals-and-sandbox", "exec", "resume", sessionID, "continue"} {
				if !slices.Contains(process.arguments, want) {
					t.Fatalf("Super resume lost argument %q", want)
				}
			}
			if local && (process.provider != "local" || process.auth["OPENAI_API_KEY"] != "godex-runtime-provider") {
				t.Fatal("local resume lost its synthetic transport")
			}
			if !slices.Equal(profiles.acquired, []string{"main"}) || profiles.released != 1 {
				t.Fatalf("profile lease = %q released=%d", profiles.acquired, profiles.released)
			}
			if got, err := os.ReadFile(filepath.Join(home, "auth.json")); err != nil || !bytes.Equal(got, auth) {
				t.Fatal("Super resume changed profile credentials")
			}
			if _, err := os.Stat(process.home); !os.IsNotExist(err) {
				t.Fatalf("Super resume left its overlay: %v", err)
			}
		})
	}
}
