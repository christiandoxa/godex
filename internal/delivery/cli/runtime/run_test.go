package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
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
	home      string
	arguments []string
}

type fakeAntigravityRunnerProcess struct {
	arguments    []string
	codexHome    string
	preparedHome string
	err          error
}

func (process *fakeAntigravityRunnerProcess) PrepareCodexHome(codexHome string) error {
	process.preparedHome = codexHome
	return nil
}

func (process *fakeAntigravityRunnerProcess) RunRuntimeWithCodexHome(_ context.Context, codexHome string, arguments []string) error {
	process.codexHome = codexHome
	process.arguments = append([]string(nil), arguments...)
	return process.err
}

func (process *fakeRunnerProcess) Run(_ context.Context, home string, arguments []string) error {
	process.home = home
	process.arguments = append([]string(nil), arguments...)
	return nil
}

type runPolicyAccounts struct {
	values []accountentity.Account
	homes  map[string]string
}

func (accounts *runPolicyAccounts) LaunchCandidates(_ context.Context, selector string) ([]accountentity.Account, error) {
	if selector == "" {
		return append([]accountentity.Account(nil), accounts.values...), nil
	}
	for _, account := range accounts.values {
		if account.ID == selector || account.Name == selector {
			return []accountentity.Account{account}, nil
		}
	}
	return nil, errors.New("account not found")
}

func (accounts *runPolicyAccounts) SelectForLaunch(_ context.Context, selector string) (accountentity.Account, error) {
	if selector == "" {
		if len(accounts.values) == 0 {
			return accountentity.Account{}, errors.New("no accounts")
		}
		return accounts.values[0], nil
	}
	for _, account := range accounts.values {
		if account.ID == selector || account.Name == selector {
			return account, nil
		}
	}
	return accountentity.Account{}, errors.New("account not found")
}

func (accounts *runPolicyAccounts) List(context.Context) ([]accountentity.Account, error) {
	return append([]accountentity.Account(nil), accounts.values...), nil
}

func (accounts *runPolicyAccounts) CodexHome(id string) string { return accounts.homes[id] }

type runPolicyQuota struct{ calls int }

func (quota *runPolicyQuota) Ready(context.Context, accountentity.Account) (bool, error) {
	quota.calls++
	return false, errors.New("quota probe should have been skipped")
}

type runBaseURLQuota struct {
	baseURL string
	noProxy bool
	calls   int
}

func (*runBaseURLQuota) Ready(context.Context, accountentity.Account) (bool, error) {
	return false, errors.New("legacy quota Ready must not handle base URL override")
}

func (quota *runBaseURLQuota) AvailabilityAtPolicy(
	_ context.Context,
	_ accountentity.Account,
	baseURL string,
	noProxy bool,
) (quotamodel.Availability, error) {
	quota.calls++
	quota.baseURL = baseURL
	quota.noProxy = noProxy
	return quotamodel.Availability{Ready: true}, nil
}

type runPolicyProcess struct {
	home      string
	arguments []string
}

func (*runPolicyProcess) Run(context.Context, string, []string) error { return nil }
func (*runPolicyProcess) CheckProxySupport(context.Context) error     { return nil }
func (process *runPolicyProcess) RunThroughProxy(_ context.Context, home, _ string, arguments []string) error {
	process.home = home
	process.arguments = append([]string(nil), arguments...)
	return nil
}

func TestProdex04356RunPolicyFlagsReachLaunchWithoutLeakingToCodex(t *testing.T) {
	root := t.TempDir()
	one := filepath.Join(root, "one")
	two := filepath.Join(root, "two")
	for _, home := range []string{one, two} {
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	accounts := &runPolicyAccounts{
		values: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": one, "two": two},
	}
	quota := &runPolicyQuota{}
	process := &runPolicyProcess{}
	var captured proxymodel.Config
	proxy := &gatewayTestProxy{}
	runner := runtimeusecase.NewRunner(accounts, process, func(config proxymodel.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return proxy, nil
	})
	runner.SetQuotaPreflight(quota)
	if err := Run(t.Context(), runner, nil, []string{
		"--account", "one",
		"--skip-quota-check",
		"--no-proxy",
		"--no-auto-rotate",
		"--full-access",
		"--",
		"exec", "hello",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if quota.calls != 0 {
		t.Fatalf("skip-quota-check still probed quota %d time(s)", quota.calls)
	}
	if !captured.SkipQuotaPreflight || !captured.UpstreamNoProxy {
		t.Fatalf("run policy did not reach proxy config: %#v", captured)
	}
	routed, err := captured.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || routed[0].ID != "one" {
		t.Fatalf("no-auto-rotate pool = %#v", routed)
	}
	joined := strings.Join(process.arguments, "\n")
	if !strings.Contains(joined, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("full-access child argv = %#v", process.arguments)
	}
	for _, forbidden := range []string{"--skip-quota-check", "--no-proxy", "--no-auto-rotate", "--full-access"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("wrapper flag %q leaked to Codex argv: %#v", forbidden, process.arguments)
		}
	}
}

func TestProdex04356RunBaseURLOverridesQuotaAndRuntimeUpstream(t *testing.T) {
	home := t.TempDir()
	accounts := &runPolicyAccounts{
		values: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:  map[string]string{"one": home},
	}
	quota := &runBaseURLQuota{}
	process := &runPolicyProcess{}
	var captured proxymodel.Config
	proxy := &gatewayTestProxy{}
	runner := runtimeusecase.NewRunner(accounts, process, func(config proxymodel.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return proxy, nil
	})
	runner.SetUpstreamURL("https://default.example.test/backend-api")
	runner.SetQuotaPreflight(quota)
	const override = "https://override.example.test/backend-api"
	if err := Run(t.Context(), runner, nil, []string{
		"--account", "one",
		"--base-url", override,
		"--no-proxy",
		"--", "exec", "hello",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if quota.calls != 1 || quota.baseURL != override || !quota.noProxy {
		t.Fatalf("quota override calls/base/no-proxy = %d / %q / %t", quota.calls, quota.baseURL, quota.noProxy)
	}
	if captured.UpstreamURL != override || !captured.UpstreamNoProxy {
		t.Fatalf("runtime upstream override = %#v", captured)
	}
}

func TestRunAndLaunchDelegateToRunner(t *testing.T) {
	accounts := &fakeRunnerAccounts{}
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	if err := Run(context.Background(), runner, nil, []string{"--account", "work", "--", "--model", "synthetic"}, io.Discard); err != nil {
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

func TestRunProfilesLaunchesAntigravityWithoutProfilesOrGeminiKey(t *testing.T) {
	process := &fakeAntigravityRunnerProcess{}
	runner := runtimeusecase.NewRunner(nil, nil, nil)
	runner.SetAntigravityProcess(process)
	home := t.TempDir()
	runner.SetAntigravityCodexHome(home)
	runner.SetAntigravitySessionLocker(codex.SessionLocker{})
	err := RunProfiles(context.Background(), runner, nil, nil, []string{
		"--provider", "gemini", "--cli", "agy", "--model", "gemini-3.1-pro", "--", "exec", "review",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--model", "gemini-3.1-pro", "--dangerously-skip-permissions", "exec", "review"}
	if !reflect.DeepEqual(process.arguments, want) {
		t.Fatalf("Antigravity args = %#v, want %#v", process.arguments, want)
	}
	if process.codexHome != home {
		t.Fatalf("Antigravity CODEX_HOME = %q", process.codexHome)
	}
}

func TestRunNativeAntigravityDryRunPrintsDiagnosticsWithoutLaunching(t *testing.T) {
	var output bytes.Buffer
	process := &fakeAntigravityRunnerProcess{}
	runner := runtimeusecase.NewRunner(nil, nil, nil)
	runner.SetAntigravityProcess(process)
	runner.SetAntigravityCodexHome("/synthetic/shared-codex")
	err := Run(context.Background(), runner, nil, []string{
		"--provider", "gemini", "--cli", "agy", "--dry-run",
	}, &output)
	if err != nil {
		t.Fatal(err)
	}
	want := "Prodex dry run: launch diagnostics\nFlow: native-cli\nProvider: antigravity\nProfile: (native CLI owned)\nRuntime proxy: disabled\n"
	if output.String() != want {
		t.Fatalf("dry-run output = %q, want %q", output.String(), want)
	}
	if process.preparedHome != "/synthetic/shared-codex" || len(process.arguments) != 0 {
		t.Fatalf("dry-run prepared %q and launched arguments %#v", process.preparedHome, process.arguments)
	}
}

func TestProdex04356RunDryRunResolvesActiveProfileWithoutLaunching(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model_provider = \"openai\"\nmodel = \"profile-model\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles := &fakeLocalLaunchProfiles{
		target: profilemodel.LaunchTarget{Name: "work", CodexHome: home, Provider: "openai"},
		active: true,
	}
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(&fakeRunnerAccounts{}, process, nil)
	var output bytes.Buffer
	err := RunProfiles(t.Context(), runner, nil, profiles, []string{
		"--dry-run", "--", "--model", "cli-model", "exec", "hello",
	}, &output)
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"Godex dry run: launch diagnostics",
		"Flow: run",
		"Binary: codex",
		"Provider: godex-openai",
		"Model: cli-model",
		"CODEX_HOME: <CODEX_HOME>",
		"Runtime proxy: would be enabled with mount /backend-api/godex",
		"Profile: (active/default)",
		"Codex/TUI not started because --dry-run was set.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, home) {
		t.Fatalf("dry-run leaked private home: %s", text)
	}
	if process.home != "" || len(process.arguments) != 0 || len(profiles.acquired) != 0 || profiles.released != 0 {
		t.Fatalf("dry-run launched/acquired: process=%q %#v leases=%v/%d", process.home, process.arguments, profiles.acquired, profiles.released)
	}
}

func TestProdex04356RunDryRunExplicitProfileRedactsSecretsAndPaths(t *testing.T) {
	home := t.TempDir()
	profiles := &fakeLocalLaunchProfiles{
		target: profilemodel.LaunchTarget{Name: "private-profile", CodexHome: home, Provider: "openai"},
	}
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(&fakeRunnerAccounts{}, process, nil)
	secret := "sk-" + strings.Repeat("x", 24)
	var output bytes.Buffer
	err := RunProfiles(t.Context(), runner, nil, profiles, []string{
		"--dry-run", "--profile", "private-profile", "--",
		"-c", "model=\"dry-model\"",
		"-c", "model_catalog_json=\"" + filepath.Join(home, "models.json") + "\"",
		"--header", "Authorization: Bearer " + secret,
	}, &output)
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{
		"Provider: godex-openai",
		"Model: dry-model",
		"Profile: <configured>",
		"<redacted-path>",
		"<redacted>",
		"Codex/TUI not started because --dry-run was set.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, text)
		}
	}
	for _, leaked := range []string{home, "private-profile", secret, "Authorization: Bearer " + secret} {
		if strings.Contains(text, leaked) {
			t.Fatalf("dry-run leaked %q:\n%s", leaked, text)
		}
	}
	if len(profiles.acquired) != 0 || profiles.released != 0 || process.home != "" || len(process.arguments) != 0 {
		t.Fatalf("dry-run caused launch side effects: leases=%v/%d process=%q %#v", profiles.acquired, profiles.released, process.home, process.arguments)
	}
}

func TestAntigravityDryRunPanelRequiresTTYAndUnsetCodexCI(t *testing.T) {
	t.Setenv("CODEX_CI", "")
	if antigravityDryRunPanelAllowed(true) {
		t.Fatal("dry-run panel enabled while CODEX_CI was set to an empty value")
	}
	if antigravityDryRunPanelAllowed(false) {
		t.Fatal("dry-run panel enabled without a TTY")
	}
	if err := os.Unsetenv("CODEX_CI"); err != nil {
		t.Fatal(err)
	}
	if !antigravityDryRunPanelAllowed(true) {
		t.Fatal("dry-run panel was disabled for a TTY with CODEX_CI unset")
	}
}

func TestAntigravityDryRunPanelUsesBubbleTea(t *testing.T) {
	var output bytes.Buffer
	t.Setenv("PRODEX_TERM_COLUMNS", "60")
	if err := printAntigravityDryRunPanel(&output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "┌"+strings.Repeat("─", 58)+"┐") ||
		!strings.Contains(output.String(), "Prodex Dry Run") ||
		!strings.Contains(output.String(), "Flow:"+strings.Repeat(" ", 10)+"native-cli") ||
		!strings.Contains(output.String(), "Provider:"+strings.Repeat(" ", 6)+"antigravity") ||
		!strings.Contains(output.String(), "Profile:"+strings.Repeat(" ", 7)+"(native CLI owned)") ||
		!strings.Contains(output.String(), "Runtime proxy: disabled") {
		t.Fatalf("dry-run Bubble Tea panel differs from tagged layout: %q", output.String())
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

func (fakeVersionedCodex) RepairSessionIndex(context.Context, string, string, string) error {
	return nil
}

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
		if err := Run(t.Context(), runner, nil, args, io.Discard); err != nil {
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
		if err := Run(t.Context(), nil, nil, args, io.Discard); err == nil || !strings.Contains(err.Error(), "bypasses Godex routing") {
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
		if err := Run(t.Context(), nil, nil, args, io.Discard); err == nil || !strings.Contains(err.Error(), "use godex") {
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
		if err := Run(t.Context(), runner, catalog, args, io.Discard); err != nil {
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
	if provider.Kind != "copilot" || provider.Name != "copilot-work" || provider.Host != host || provider.Login != login || provider.APIURL != apiURL || provider.DefaultModel != "gpt-6-astra" || provider.ContextWindow != 1050000 || provider.AutoCompactLimit != 997500 {
		t.Fatalf("Copilot provider = %#v", provider)
	}
	openAI, err := launchRuntimeProvider(profilemodel.LaunchTarget{Provider: "openai"})
	if err != nil || openAI.Kind != "" {
		t.Fatalf("OpenAI provider = %#v, err = %v", openAI, err)
	}
	anthropic, err := launchRuntimeProvider(profilemodel.LaunchTarget{Name: "claude-work", Provider: "anthropic"})
	if err != nil || anthropic.Kind != "anthropic" || anthropic.Name != "claude-work" || anthropic.APIURL != "https://api.anthropic.com/v1" || anthropic.DefaultModel != "claude-sonnet-5-5" || anthropic.ContextWindow != 1000000 || anthropic.AutoCompactLimit != 950000 {
		t.Fatalf("Anthropic provider = %#v, err = %v", anthropic, err)
	}
	kiro, err := launchRuntimeProvider(profilemodel.LaunchTarget{Name: "kiro-work", Provider: "kiro"})
	if err != nil || kiro.Kind != "kiro" || kiro.Name != "kiro-work" || kiro.APIURL != "https://kiro.dev" || kiro.DefaultModel != "auto" || kiro.ContextWindow != 1_000_000 || kiro.AutoCompactLimit != 950_000 {
		t.Fatalf("Kiro provider = %#v, err = %v", kiro, err)
	}
}

type fakeLocalLaunchProfiles struct {
	target   profilemodel.LaunchTarget
	active   bool
	acquired []string
	released int
}

func (fake *fakeLocalLaunchProfiles) ResolveLaunch(_ context.Context, name string) (profilemodel.LaunchTarget, error) {
	if fake.target.Name != name {
		return profilemodel.LaunchTarget{}, errors.New("missing profile")
	}
	return fake.target, nil
}

func (fake *fakeLocalLaunchProfiles) ActiveLaunch(context.Context) (profilemodel.LaunchTarget, bool, error) {
	return fake.target, fake.active, nil
}

func (fake *fakeLocalLaunchProfiles) CurrentLaunch(context.Context) (profilemodel.LaunchTarget, error) {
	if !fake.active {
		return profilemodel.LaunchTarget{}, errors.New("no active profile")
	}
	return fake.target, nil
}

func (fake *fakeLocalLaunchProfiles) AcquireLaunch(_ context.Context, name string) (func() error, error) {
	fake.acquired = append(fake.acquired, name)
	return func() error {
		fake.released++
		return nil
	}, nil
}

func (fake *fakeLocalLaunchProfiles) ProviderLaunchPool(context.Context, string, string, bool) ([]profilemodel.LaunchTarget, error) {
	return nil, errors.New("provider pool must not run for --url")
}

func (fake *fakeLocalLaunchProfiles) ResolveProviderLaunch(context.Context, string, string) (profilemodel.LaunchTarget, bool, error) {
	return profilemodel.LaunchTarget{}, false, errors.New("provider resolution must not run for --url")
}

func (fake *fakeLocalLaunchProfiles) AcquireLaunchPool(context.Context, []string) (func() error, error) {
	return nil, errors.New("provider pool lease must not run for --url")
}

func (fake *fakeLocalLaunchProfiles) OpenAICompatibleBaseURL(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func TestRunHomeLocalProviderKeepsResolvedHome(t *testing.T) {
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(&fakeRunnerAccounts{}, process, nil)
	home := t.TempDir()
	if err := RunHome(context.Background(), runner, nil, home, []string{
		"--url", "http://127.0.0.1:8131", "--model", "qwen3-coder", "exec", "review",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if process.home != home {
		t.Fatalf("local provider home = %q, want %q", process.home, home)
	}
	joined := strings.Join(process.arguments, "\n")
	for _, expected := range []string{
		"model_provider=\"godex-local\"",
		"model=\"qwen3-coder\"",
		"model_providers.godex-local.base_url=\"http://127.0.0.1:8131/v1\"",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("local provider args missing %q: %#v", expected, process.arguments)
		}
	}
}

func TestRunProfilesLocalProviderUsesExplicitStandaloneProfileLease(t *testing.T) {
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(&fakeRunnerAccounts{}, process, nil)
	home := t.TempDir()
	profiles := &fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
		Name: "local-home", CodexHome: home,
	}}
	if err := RunProfiles(context.Background(), runner, nil, profiles, []string{
		"--profile", "local-home", "--url", "http://127.0.0.1:8131", "exec",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if process.home != home || strings.Join(profiles.acquired, ",") != "local-home" || profiles.released != 1 {
		t.Fatalf("home/lease = %q / %v / %d", process.home, profiles.acquired, profiles.released)
	}
}

func TestRunProfilesLocalProviderUsesActiveStandaloneProfile(t *testing.T) {
	process := &fakeRunnerProcess{}
	runner := runtimeusecase.NewRunner(&fakeRunnerAccounts{}, process, nil)
	home := t.TempDir()
	profiles := &fakeLocalLaunchProfiles{
		target: profilemodel.LaunchTarget{Name: "active-local", CodexHome: home},
		active: true,
	}
	if err := RunProfiles(context.Background(), runner, nil, profiles, []string{
		"--url=http://127.0.0.1:8131", "exec",
	}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if process.home != home || strings.Join(profiles.acquired, ",") != "active-local" || profiles.released != 1 {
		t.Fatalf("active home/lease = %q / %v / %d", process.home, profiles.acquired, profiles.released)
	}
}

type sharedNativeSessionAccounts struct{}

func (sharedNativeSessionAccounts) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{
		{ID: "one", Name: "personal", Enabled: true},
		{ID: "two", Name: "work", Enabled: true},
	}, nil
}

func (sharedNativeSessionAccounts) CodexHome(id string) string {
	if id == "two" {
		return "/profiles/two"
	}
	return "/profiles/one"
}

func (sharedNativeSessionAccounts) LaunchCandidates(context.Context, string) ([]accountentity.Account, error) {
	return nil, errors.New("fresh launch selection must not run for resolved shared sessions")
}

func (sharedNativeSessionAccounts) SelectForLaunch(context.Context, string) (accountentity.Account, error) {
	return accountentity.Account{}, errors.New("fresh launch selection must not run for resolved shared sessions")
}

type sharedNativeSessionReader struct{ project string }

func (reader sharedNativeSessionReader) List(_ context.Context, home string) ([]sessionentity.Session, error) {
	if home == "/profiles/two" {
		return []sessionentity.Session{{
			ID: "00000000-0000-4000-8000-000000000222", ThreadName: "Repair", CWD: reader.project,
			Source: "cli", UpdatedUnix: 20, Path: "/profiles/two/sessions/repair.jsonl",
		}}, nil
	}
	return []sessionentity.Session{{
		ID: "00000000-0000-4000-8000-000000000111", ThreadName: "Older", CWD: reader.project,
		Source: "cli", UpdatedUnix: 10, Path: "/profiles/one/sessions/older.jsonl",
	}}, nil
}

func TestNativeSessionDeliveryRoutesNameAndLastAcrossProfileHomes(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	for _, args := range [][]string{
		{"--", "resume", "Repair", "continue"},
		{"--", "resume", "--last", "continue"},
		{"--", "fork", "--last", "continue"},
	} {
		accounts := sharedNativeSessionAccounts{}
		process := &fakeRunnerProcess{}
		runner := runtimeusecase.NewRunner(accounts, process, nil)
		catalog := sessionusecase.NewCatalog(accounts, sharedNativeSessionReader{project: project}, runner)
		if err := Run(t.Context(), runner, catalog, args, io.Discard); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if process.home != "/profiles/two" || !strings.Contains(strings.Join(process.arguments, " "), "00000000-0000-4000-8000-000000000222") {
			t.Fatalf("shared session route for %v = home %q args %#v", args, process.home, process.arguments)
		}
	}
}

func TestProdex04356BareSessionSelectorSurvivesRootConfigOptions(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	index, args := sessionArgument([]string{"-c", "features.apps=false", id, "continue"})
	if index != 3 {
		t.Fatalf("session index = %d, args = %#v", index, args)
	}
	want := []string{"-c", "features.apps=false", "resume", id, "continue"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("session rewrite = %#v, want %#v", args, want)
	}
}
