package runtime

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

func superResumeSessions(t *testing.T, provider, model, effort string) (*sessionusecase.Catalog, string) {
	t.Helper()
	const sessionID = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44fb"
	return sessionusecase.NewCatalog(
		&fakeRunnerAccounts{},
		resumeSessionReader{values: []sessionentity.Session{{
			ID: sessionID, ModelProvider: provider, LastModel: model, LastReasoningEffort: effort,
			UpdatedUnix: 10, Path: "/shared/sessions/super-resume.jsonl",
		}}},
		nil,
	), sessionID
}

func TestProdex04357SuperResumeRestoresProviderModelAndEffortAtRuntime(t *testing.T) {
	sessions, sessionID := superResumeSessions(t, "godex-kiro", "gpt-5.6-luna", "max")
	home := t.TempDir()
	profiles := &kiroShortcutProfiles{target: profilemodel.LaunchTarget{
		Name: "kiro-main", CodexHome: home, Provider: "kiro",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "kiro"},
	}}
	process := &kiroShortcutProcess{}
	runner := runtimeusecase.NewRunner(nil, process, func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		return &kiroShortcutProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))
	lookup := func(string) (string, bool) { return "", false }
	var output bytes.Buffer
	if err := superProfilesWithToolLookup(t.Context(), runner, sessions, profiles, &output, []string{
		"--no-sub-agent", sessionID,
	}, lookup); err != nil {
		t.Fatal(err)
	}
	if process.provider != "kiro" || process.home == "" {
		t.Fatalf("super resume provider/home = %q / %q", process.provider, process.home)
	}
	joined := strings.Join(process.arguments, "\n")
	for _, want := range []string{
		"model=\"gpt-5.6-luna\"",
		"model_reasoning_effort=\"max\"",
		"--dangerously-bypass-approvals-and-sandbox",
		"resume", sessionID,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("super resume args missing %q: %#v", want, process.arguments)
		}
	}
}

func TestProdex04357SuperResumeDryRunRestoresProviderModelAndEffort(t *testing.T) {
	sessions, sessionID := superResumeSessions(t, "prodex-kiro", "gpt-5.6-luna", "max")
	lookup := func(string) (string, bool) { return "", false }
	var output bytes.Buffer
	if err := superProfilesWithToolLookup(t.Context(), nil, sessions, nil, &output, []string{
		"--dry-run", "--no-sub-agent", sessionID,
	}, lookup); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"Provider: kiro",
		"model=\"gpt-5.6-luna\"",
		"model_reasoning_effort=\"max\"",
		"<SESSION_UUID>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("super resume dry-run missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, sessionID) {
		t.Fatalf("super resume dry-run leaked raw parent session UUID: %s", text)
	}
}

func TestProdex04357SuperResumeExplicitProviderAndModelWin(t *testing.T) {
	sessions, sessionID := superResumeSessions(t, "godex-kiro", "session-model", "high")
	options, err := parseSuperArguments([]string{
		"--provider", "deepseek", "--model", "explicit-model", sessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveSuperResumeOptions(t.Context(), sessions, options)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.provider != "deepseek" || resolved.model != "explicit-model" {
		t.Fatalf("explicit super provider/model = %q / %q", resolved.provider, resolved.model)
	}
	joined := strings.Join(resolved.codexArgs, "\n")
	if strings.Contains(joined, "session-model") {
		t.Fatalf("session model overrode explicit Super model: %#v", resolved.codexArgs)
	}
	if !strings.Contains(joined, "model_reasoning_effort=\"high\"") {
		t.Fatalf("session effort was not restored: %#v", resolved.codexArgs)
	}
}

func TestProdex04357SuperResumeUnknownProviderFailsClosed(t *testing.T) {
	const secretLike = "unknown-provider-super-secret"
	sessions, sessionID := superResumeSessions(t, secretLike, "model", "high")
	options, err := parseSuperArguments([]string{sessionID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveSuperResumeOptions(t.Context(), sessions, options)
	if err == nil || !strings.Contains(err.Error(), "unsupported provider identity") {
		t.Fatalf("unknown Super resume provider error = %v", err)
	}
	if strings.Contains(err.Error(), secretLike) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("unknown Super resume provider leaked identity: %v", err)
	}
}
