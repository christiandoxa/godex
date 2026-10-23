package runtime

import (
	"context"
	"io"
	"path/filepath"
	"slices"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type profileSettingsReader04390 struct {
	home, provider string
}

func (reader profileSettingsReader04390) List(_ context.Context, home string) ([]sessionentity.Session, error) {
	if home != reader.home {
		return nil, nil
	}
	return []sessionentity.Session{{
		ID: legacySettingsSession04390, Source: "cli", ModelProvider: reader.provider,
		LastModel: "synthetic-saved-model", LastReasoningEffort: "high",
		Path: filepath.Join(home, "sessions", "rollout.jsonl"),
	}}, nil
}

const legacySettingsSession04390 = "01900000-0000-7000-8000-000000000165"

func TestProdex04390ProfileOnlyResumeLeavesSavedSettingsToNativeCodex(t *testing.T) {
	for _, provider := range []string{"openai", "prodex-kiro", "synthetic-unknown"} {
		t.Run(provider, func(t *testing.T) {
			home := t.TempDir()
			profiles := &superResumeProfiles04390{fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
				Name: "main", CodexHome: home, Provider: "openai", Auth: "api-key",
			}}}
			catalog := sessionusecase.NewCatalog(&fakeRunnerAccounts{}, profileSettingsReader04390{home: home, provider: provider}, nil)
			catalog.SetSharedCodexHome(t.TempDir())
			catalog.SetProfiles(profiles)
			nativeArgs := []string{"exec", "resume", legacySettingsSession04390, "continue"}
			t.Run("run", func(t *testing.T) {
				process := &fakeRunnerProcess{}
				runner := runtimeusecase.NewRunner(nil, process, nil)
				args := append([]string{"--profile", "main", "--skip-quota-check", "--no-auto-rotate"}, nativeArgs...)
				if err := RunProfiles(t.Context(), runner, catalog, profiles, args, io.Discard); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(process.arguments, nativeArgs) {
					t.Fatalf("profile-only resume invented native overrides: %q", process.arguments)
				}
			})
			for _, local := range []bool{false, true} {
				name := "super"
				if local {
					name = "super_local"
				}
				t.Run(name, func(t *testing.T) {
					args := []string{"--profile", "main"}
					if local {
						args = append(args, "--url", "http://127.0.0.1:11434", "--model", "explicit-local")
					}
					options, err := parseSuperArguments(append(args, nativeArgs...))
					if err != nil {
						t.Fatal(err)
					}
					resolved, err := resolveSuperResumeOptions(t.Context(), catalog, options)
					if err != nil {
						t.Fatal(err)
					}
					if resolved.provider != "" || !slices.Equal(resolved.codexArgs, options.codexArgs) || resolved.model != options.model {
						t.Fatalf("profile-only Super changed provider/model/native settings: provider=%q args=%q", resolved.provider, resolved.codexArgs)
					}
				})
			}
		})
	}
}
