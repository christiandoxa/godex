package runtime

import (
	"context"
	"io"
	"strings"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type resumeSessionReader struct{ values []sessionentity.Session }

func (reader resumeSessionReader) List(context.Context, string) ([]sessionentity.Session, error) {
	return append([]sessionentity.Session(nil), reader.values...), nil
}

func TestProdex04356ResumeReconstructsKiroProviderAndSessionSettings(t *testing.T) {
	const sessionID = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"
	sessions := sessionusecase.NewCatalog(&fakeRunnerAccounts{}, resumeSessionReader{values: []sessionentity.Session{{
		ID: sessionID, ModelProvider: "godex-kiro", LastModel: "gpt-5.6-luna", LastReasoningEffort: "max",
		UpdatedUnix: 10, Path: "/shared/sessions/provider.jsonl",
	}}}, nil)
	home := t.TempDir()
	profiles := &kiroShortcutProfiles{target: profilemodel.LaunchTarget{Name: "kiro-main", CodexHome: home, Provider: "kiro", ProviderConfig: profilemodel.ProviderSnapshot{Kind: "kiro"}}}
	process := &kiroShortcutProcess{}
	var captured proxyconfig.Config
	runner := runtimeusecase.NewRunner(nil, process, func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return &kiroShortcutProxy{}, nil
	})
	if err := RunProfiles(t.Context(), runner, sessions, profiles, []string{"resume", sessionID}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if process.provider != "kiro" || process.home != home {
		t.Fatalf("resume provider/home = %q / %q", process.provider, process.home)
	}
	joined := strings.Join(process.arguments, "\n")
	for _, want := range []string{"model=\"gpt-5.6-luna\"", "model_reasoning_effort=\"max\"", "model_reasoning_summary=\"none\"", "web_search=\"live\"", "features.apps=false", "features.js_repl=false", "features.image_generation=false", "resume", sessionID} {
		if !strings.Contains(joined, want) {
			t.Fatalf("resume args missing %q: %#v", want, process.arguments)
		}
	}
	if captured.Provider.Kind != "kiro" {
		t.Fatalf("resume proxy provider = %#v", captured.Provider)
	}
}

func TestProdex04356ResumeSessionSettingsRespectExplicitOverrides(t *testing.T) {
	report := sessionmodel.Report{LastModel: "session-model", LastReasoningEffort: "high"}
	got := restoreResumeSessionSettings([]string{"-m", "explicit-model", "-c", "model_reasoning_effort=\"low\"", "resume", "019c9e3d"}, report)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "session-model") || strings.Contains(joined, "\"high\"") {
		t.Fatalf("explicit resume settings were overwritten: %#v", got)
	}
	if !strings.Contains(joined, "explicit-model") || !strings.Contains(joined, "\"low\"") {
		t.Fatalf("explicit resume settings missing: %#v", got)
	}
}

func TestProdex04356ResumeProviderIdentityFailsClosedAndKeepsBedrockDirect(t *testing.T) {
	for _, provider := range []string{"amazon-bedrock", "amazon-bedrock-runtime"} {
		plan, err := resumeProviderIdentity(provider)
		if err != nil || !plan.direct || plan.kind != "" {
			t.Fatalf("Bedrock %q plan = %#v err=%v", provider, plan, err)
		}
	}
	const secretLikeUnknown = "unknown-provider-sk-super-secret"
	_, err := resumeProviderIdentity(secretLikeUnknown)
	if err == nil || !strings.Contains(err.Error(), "unsupported provider identity") {
		t.Fatalf("unknown provider error = %v", err)
	}
	if strings.Contains(err.Error(), secretLikeUnknown) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("unknown provider leaked identity: %v", err)
	}
}

func TestProdex04356ResumeProviderIdentityAcceptsTaggedCompatibilityAliases(t *testing.T) {
	for provider, want := range map[string]string{"prodex-anthropic": "anthropic", "prodex-copilot": "copilot", "prodex-deepseek": "deepseek", "prodex-gemini": "gemini", "prodex-kiro": "kiro"} {
		plan, err := resumeProviderIdentity(provider)
		if err != nil || plan.kind != want || plan.direct {
			t.Fatalf("%q => %#v err=%v", provider, plan, err)
		}
	}
}
