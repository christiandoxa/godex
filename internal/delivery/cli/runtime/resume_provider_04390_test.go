package runtime

import (
	"io"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func TestProdex04390ResumeProviderRegistryAliases(t *testing.T) {
	for kind, aliases := range map[string][]string{
		"":          {"openai", "openai-responses", "openai-compatible", "openai_compatible", "prodex-openai-governed-http", "local", "local-openai", "local_openai", "prodex-local"},
		"anthropic": {"anthropic", "claude", "prodex-anthropic"},
		"copilot":   {"copilot", "github-copilot", "github_copilot", "prodex-copilot"},
		"deepseek":  {"deepseek", "prodex-deepseek"},
		"gemini":    {"gemini", "google", "prodex-gemini"},
		"kiro":      {"kiro", "prodex-kiro"},
	} {
		for _, alias := range aliases {
			for _, value := range []string{alias, "\u2003" + strings.ToUpper(alias) + "\u00a0"} {
				plan, err := resumeProviderIdentity(value)
				if err != nil || plan.kind != kind || plan.direct {
					t.Errorf("provider %q: kind=%q direct=%t err=%v", value, plan.kind, plan.direct, err)
				}
			}
		}
	}
	for _, value := range []string{"prodex-openai", "prodex-openai-compatible", "prodex-\u212airo", "\u212airo"} {
		if _, err := resumeProviderIdentity(value); err == nil {
			t.Errorf("accepted unsupported registry identity %q", value)
		}
	}
	for _, value := range []string{"godex-openai", "godex-openai-compatible", "godex-openai-governed-http", "godex-local"} {
		if _, err := resumeProviderIdentity(value); err != nil {
			t.Errorf("lost Godex's persisted provider compatibility for %q: %v", value, err)
		}
	}
}

func TestProdex04390ExplicitProfileRejectsUnknownSharedSessionProvider(t *testing.T) {
	for _, fixture := range []struct {
		provider string
		override bool
		wantErr  bool
	}{
		{"synthetic-private-identity", false, true},
		{"prodex-openai", false, true},
		{"prodex-openai-compatible", false, true},
		{"prodex-\u212airo", false, true},
		{"synthetic-private-identity", true, false},
		{"openai-compatible", false, false},
		{"prodex-openai-governed-http", false, false},
	} {
		t.Run(fixture.provider, func(t *testing.T) {
			home, shared := t.TempDir(), t.TempDir()
			profiles := &superResumeProfiles04390{fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
				Name: "main", CodexHome: home, Provider: "openai", Auth: "api-key",
			}}}
			catalog := sessionusecase.NewCatalog(&fakeRunnerAccounts{}, profileSettingsReader04390{home: shared, provider: fixture.provider}, nil)
			catalog.SetSharedCodexHome(shared)
			catalog.SetProfiles(profiles)
			process := &fakeRunnerProcess{}
			runner := runtimeusecase.NewRunner(nil, process, nil)
			args := []string{"--profile", "main", "--skip-quota-check", "--no-auto-rotate", "exec", "resume"}
			if fixture.override {
				args = append(args, "-c", `model_provider="openai"`)
			}
			args = append(args, legacySettingsSession04390, "continue")
			err := RunProfiles(t.Context(), runner, catalog, profiles, args, io.Discard)
			if (err != nil) != fixture.wantErr {
				t.Fatalf("provider validation error=%v wantErr=%t", err, fixture.wantErr)
			}
			if fixture.wantErr {
				if !strings.Contains(err.Error(), "unsupported provider identity") || strings.Contains(err.Error(), fixture.provider) || process.home != "" || len(profiles.acquired) != 0 {
					t.Fatal("unknown provider reached profile launch or exposed its identity")
				}
			} else if process.home != home || len(profiles.acquired) != 1 || profiles.released != 1 {
				t.Fatal("accepted native provider lost the selected profile or lease")
			}
		})
	}
}
