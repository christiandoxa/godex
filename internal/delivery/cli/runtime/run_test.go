package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type fakeRunnerAccounts struct {
	selected string
}

func (accounts *fakeRunnerAccounts) LaunchCandidates(_ context.Context, selector string) ([]accountentity.Account, error) {
	return []accountentity.Account{{ID: "synthetic-account", Name: selector, Enabled: true}}, nil
}

func (accounts *fakeRunnerAccounts) SelectForLaunch(_ context.Context, selector string) (accountentity.Account, error) {
	accounts.selected = selector
	return accountentity.Account{ID: "synthetic-account", Enabled: true}, nil
}

func (fakeRunnerAccounts) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{{ID: "synthetic-account", Enabled: true}}, nil
}

func (fakeRunnerAccounts) CodexHome(string) string { return "/synthetic/codex" }

type fakeRunnerProcess struct {
	arguments []string
}

func (process *fakeRunnerProcess) Run(_ context.Context, _ string, arguments []string) error {
	process.arguments = append([]string(nil), arguments...)
	return nil
}

func TestRunAndLaunchDelegateToRunner(t *testing.T) {
	accounts := &fakeRunnerAccounts{}
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	if err := Run(context.Background(), runner, nil, []string{"--account", "work", "--", "--model", "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if accounts.selected != "work" || strings.Join(process.arguments, " ") != "--model synthetic" {
		t.Fatalf("runner call = selector %q, arguments %#v", accounts.selected, process.arguments)
	}
	if err := Launch(context.Background(), runner); err != nil {
		t.Fatal(err)
	}
	if accounts.selected != "" {
		t.Fatalf("launch selector = %q", accounts.selected)
	}
}

type fakeDoctorAccounts struct{}

func (fakeDoctorAccounts) Prepare() error { return nil }
func (fakeDoctorAccounts) Root() string   { return "/synthetic/godex" }
func (fakeDoctorAccounts) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{{Enabled: true}, {Enabled: false}}, nil
}

type fakeVersionedCodex struct{}

func (fakeVersionedCodex) Version(context.Context) (string, error) { return "codex synthetic", nil }

func (fakeVersionedCodex) CheckProxySupport(context.Context) error { return nil }

func TestDoctorRendersReport(t *testing.T) {
	doctor := runtimeusecase.NewDoctor(fakeDoctorAccounts{}, fakeVersionedCodex{})
	var output strings.Builder
	if err := Doctor(context.Background(), doctor, &output, nil); err != nil {
		t.Fatal(err)
	}
	want := "Doctor\nGodex root: /synthetic/godex\nCodex: codex synthetic\nAccounts: 2 (1 enabled)\n"
	if output.String() != want {
		t.Fatalf("doctor output = %q", output.String())
	}
	if err := Doctor(context.Background(), doctor, &output, []string{"extra"}); err == nil {
		t.Fatal("doctor accepted an unknown argument")
	}
	if err := Doctor(context.Background(), doctor, failingWriter{}, nil); err == nil {
		t.Fatal("doctor output failure was ignored")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic output failure") }

func (accounts *fakeRunnerAccounts) Current(context.Context) (accountentity.Account, error) {
	return accountentity.Account{ID: "synthetic-account", Enabled: true}, nil
}
func (accounts *fakeRunnerAccounts) Resolve(ctx context.Context, selector string) (accountentity.Account, error) {
	return accounts.Current(ctx)
}

func TestNativeLocalDispatchRecognizesRootAndWrapperOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--current-time-reminder", "features", "list"},
		{"--", "--model", "synthetic", "mcp", "list"},
		{"--", "--config=model=synthetic", "login", "status"},
		{"--", "--version"},
	} {
		accounts := &fakeRunnerAccounts{selected: "unchanged"}
		process := &fakeRunnerProcess{}
		runner := runtimeusecase.NewRunner(accounts, process, nil)
		if err := Run(t.Context(), runner, nil, args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if accounts.selected != "unchanged" || len(process.arguments) == 0 {
			t.Fatalf("local command used fresh selection: %v", args)
		}
	}
}

func TestNativeCommandsCannotDiscardManagedRouting(t *testing.T) {
	for _, args := range [][]string{
		{"debug", "app-server", "send-message-v2", "message"},
		{"--current-time-reminder", "debug", "-c", "model=synthetic", "app-server", "send-message-v2", "message"},
		{"--", "--model", "synthetic", "app-server", "proxy"},
		{"app-server", "daemon", "start"},
		{"app-server", "--listen", "stdio", "proxy"},
		{"app-server", "--ws-auth", "capability-token", "--ws-token-file", "/synthetic/token", "proxy"},
	} {
		if err := Run(t.Context(), nil, nil, args); err == nil || !strings.Contains(err.Error(), "bypasses Godex routing") {
			t.Fatalf("unsafe native command accepted: %v, %v", args, err)
		}
	}
	for _, args := range [][]string{{"debug", "models"}, {"exec", "debug app-server"}, {"--", "debug", "app-server"}, {"app-server", "generate-json-schema"}} {
		index := nativeCommandIndex(args)
		if index >= 0 && unsafeNativeCommand(args, index) {
			t.Fatalf("safe native command rejected: %v", args)
		}
	}
}

func TestPassthroughCannotMutateManagedCredentials(t *testing.T) {
	for _, args := range [][]string{{"--", "logout"}, {"--current-time-reminder", "logout"}, {"--", "--model", "synthetic", "login"}, {"--", "login", "--device-auth"}} {
		if err := Run(t.Context(), nil, nil, args); err == nil || !strings.Contains(err.Error(), "use godex") {
			t.Fatalf("unsafe auth passthrough %v: %v", args, err)
		}
	}
}

type nativeSessionReader struct{}

func (nativeSessionReader) List(context.Context, string) ([]sessionentity.Session, error) {
	return []sessionentity.Session{{ID: "00000000-0000-4000-8000-000000000001"}}, nil
}

func TestNativeSessionDeliveryPreservesOptionsAndResolvesPrefix(t *testing.T) {
	for _, args := range [][]string{
		{"--", "--model", "synthetic", "exec", "--json", "resume", "0000", "prompt"},
		{"--", "--model", "synthetic", "exec", "fork", "0000", "prompt"},
		{"--", "-c", "model=synthetic", "delete", "--force", "0000"},
		{"--", "queue", "--thread", "0000", "--message", "message"},
		{"--", "queue", "--message", "message", "--thread=0000"},
	} {
		accounts := &fakeRunnerAccounts{selected: "unchanged"}
		process := &fakeRunnerProcess{}
		runner := runtimeusecase.NewRunner(accounts, process, nil)
		catalog := sessionusecase.NewCatalog(accounts, nativeSessionReader{}, runner)
		if err := Run(t.Context(), runner, catalog, args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if accounts.selected != "unchanged" || !strings.Contains(strings.Join(process.arguments, " "), "00000000-0000-4000-8000-000000000001") {
			t.Fatalf("native session lost its owner: %v", process.arguments)
		}
	}
}

func TestLaunchRuntimeProviderSupportsCopilotAndAnthropic(t *testing.T) {
	host, login := "https://github.com", "octocat"
	apiURL := "https://api.githubcopilot.com"
	provider, err := launchRuntimeProvider(profilemodel.LaunchTarget{
		Name: "copilot-work", Provider: "copilot",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "copilot", Host: &host, Login: &login, APIURL: &apiURL},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Kind != "copilot" || provider.Name != "copilot-work" || provider.Host != host || provider.Login != login || provider.APIURL != apiURL || provider.DefaultModel != "gpt-5.3-codex" || provider.ContextWindow != 272000 || provider.AutoCompactLimit != 258400 {
		t.Fatalf("Copilot provider = %#v", provider)
	}
	openAI, err := launchRuntimeProvider(profilemodel.LaunchTarget{Provider: "openai"})
	if err != nil || openAI.Kind != "" {
		t.Fatalf("OpenAI provider = %#v, err = %v", openAI, err)
	}
	anthropic, err := launchRuntimeProvider(profilemodel.LaunchTarget{Name: "claude-work", Provider: "anthropic"})
	if err != nil || anthropic.Kind != "anthropic" || anthropic.Name != "claude-work" || anthropic.APIURL != "https://api.anthropic.com/v1" || anthropic.DefaultModel != "claude-sonnet-4-6" || anthropic.ContextWindow != 200000 || anthropic.AutoCompactLimit != 180000 {
		t.Fatalf("Anthropic provider = %#v, err = %v", anthropic, err)
	}
	if _, err := launchRuntimeProvider(profilemodel.LaunchTarget{Provider: "kiro"}); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unsupported provider error = %v", err)
	}
}
