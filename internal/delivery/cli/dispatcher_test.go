package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authcli "github.com/christiandoxa/godex/internal/delivery/cli/auth"
	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profileentity "github.com/christiandoxa/godex/internal/entity/profile"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	updategateway "github.com/christiandoxa/godex/internal/gateway/update"
	authmodel "github.com/christiandoxa/godex/internal/model/auth"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
	updaterepo "github.com/christiandoxa/godex/internal/repository/update"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	updateusecase "github.com/christiandoxa/godex/internal/usecase/update"
)

type failingWriter struct{ err error }

func (writer failingWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestDispatcherVersion(t *testing.T) {
	var output bytes.Buffer
	if err := New(nil, nil, nil, nil, nil, nil, &output).Run(context.Background(), []string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "godex") {
		t.Fatalf("version output = %q", output.String())
	}
}

func TestDispatcherPassesUnknownCommandToCodex(t *testing.T) {
	accounts := dispatcherAccounts{}
	process := &dispatcherProcess{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	if err := New(nil, nil, accounts, runner, nil, nil, &bytes.Buffer{}).Run(context.Background(), []string{"remote-control", "--help"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(process.arguments, " "); got != "remote-control --help" {
		t.Fatalf("passthrough arguments = %q", got)
	}
}

func TestDispatcherReturnsHelpOutputError(t *testing.T) {
	errSynthetic := errors.New("synthetic output failure")
	err := New(nil, nil, nil, nil, nil, nil, failingWriter{err: errSynthetic}).Run(context.Background(), []string{"help"})
	if !errors.Is(err, errSynthetic) {
		t.Fatalf("help error = %v", err)
	}
}

type dispatcherAccounts struct{}

func (dispatcherAccounts) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{{ID: "synthetic", Name: "work", Email: "work@example.com", Enabled: true}}, nil
}

func (dispatcherAccounts) Current(context.Context) (accountentity.Account, error) {
	return accountentity.Account{ID: "synthetic", Name: "work", Email: "<redacted>", Enabled: true}, nil
}

func (dispatcherAccounts) SetActive(_ context.Context, selector string) (accountentity.Account, error) {
	return accountentity.Account{Name: selector, Enabled: true}, nil
}

func (dispatcherAccounts) Remove(_ context.Context, selector string) (accountentity.Account, error) {
	return accountentity.Account{Name: selector, Enabled: true}, nil
}

func (dispatcherAccounts) Prepare() error { return nil }
func (dispatcherAccounts) Root() string   { return "/synthetic/godex" }

func (dispatcherAccounts) LaunchCandidates(_ context.Context, selector string) ([]accountentity.Account, error) {
	return []accountentity.Account{{ID: "synthetic", Name: selector, Enabled: true}}, nil
}

func (dispatcherAccounts) SelectForLaunch(_ context.Context, selector string) (accountentity.Account, error) {
	return accountentity.Account{ID: "synthetic", Name: selector, Enabled: true}, nil
}

func (dispatcherAccounts) CodexHome(string) string { return "/synthetic/codex" }

type dispatcherProcess struct {
	home      string
	arguments []string
	called    bool
}

func (process *dispatcherProcess) Run(_ context.Context, home string, arguments []string) error {
	process.called = true
	process.home = home
	process.arguments = append([]string(nil), arguments...)
	return nil
}

func (dispatcherProcess) Version(context.Context) (string, error) { return "codex synthetic", nil }

func (dispatcherProcess) CheckProxySupport(context.Context) error { return nil }

type dispatcherLoginAccounts struct{}

func (dispatcherLoginAccounts) CreateStagedHome() (string, error) { return "/synthetic/staged", nil }
func (dispatcherLoginAccounts) RemoveStagedHome(string) error     { return nil }
func (dispatcherLoginAccounts) CommitLogin(_ context.Context, candidate accountentity.Account, _ string, _ bool) (accountentity.Account, error) {
	return candidate, nil
}

func (dispatcherLoginAccounts) CommitImportCurrent(_ context.Context, candidate accountentity.Account, _ string, complete func() error) (accountentity.Account, error) {
	if complete != nil {
		if err := complete(); err != nil {
			return accountentity.Account{}, err
		}
	}
	return candidate, nil
}

func (dispatcherLoginAccounts) List(context.Context) ([]accountentity.Account, error) {
	return nil, nil
}

type dispatcherLoginCodex struct{}

func (dispatcherLoginCodex) Login(context.Context, string, bool) (accountentity.Identity, error) {
	return accountentity.Identity{Email: "login@example.com", ChatGPTAccountID: "login-account"}, nil
}

func (dispatcherLoginCodex) StageImportCurrentAuth(context.Context, string, string, bool) (authmodel.ImportCurrentIdentity, error) {
	return authmodel.ImportCurrentIdentity{Email: "<redacted>", ChatGPTAccountID: "<redacted>"}, nil
}

func (dispatcherLoginCodex) CompleteImportCurrentHome(context.Context, string, string) error {
	return nil
}

func TestDispatcherRoutesCommandDomains(t *testing.T) {
	accounts := dispatcherAccounts{}
	process := &dispatcherProcess{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	doctor := runtimeusecase.NewDoctor(accounts, process)
	login := authusecase.NewLogin(dispatcherLoginAccounts{}, dispatcherLoginCodex{})
	importer := authusecase.NewImportCurrent(dispatcherLoginAccounts{}, dispatcherLoginCodex{}, "/synthetic/current")
	var output bytes.Buffer
	app := New(login, importer, accounts, runner, doctor, nil, &output)
	commands := [][]string{
		{"accounts"}, {"current"}, {"account", "list"}, {"profile", "current"}, {"account", "use", "work"}, {"account", "remove", "work"}, {"profile", "import-current", "imported"},
		{"use", "work"}, {"remove", "work"}, {"run", "--account", "work", "--", "--model", "synthetic"},
		{"login", menuLoginNameOption, "work"}, {}, {"doctor"},
	}
	for _, command := range commands {
		if err := app.Run(context.Background(), command); err != nil {
			t.Fatalf("command %#v: %v", command, err)
		}
	}
	if !strings.Contains(output.String(), "work") || !process.called {
		t.Fatalf("dispatcher output/arguments = %q, %#v", output.String(), process.arguments)
	}
}

func TestDispatcherImportCurrentCopiesNativeHomeAndActivates(t *testing.T) {
	accounts := accountrepo.NewFileStore(t.TempDir())
	source := t.TempDir()
	if err := os.Chmod(source, 0o700); err != nil {
		t.Fatal(err)
	}
	authJSON := `{"auth_mode":"chatgpt","tokens":{"access_token":"opaque","account_id":"dispatcher-account"}}`
	if err := os.WriteFile(filepath.Join(source, "auth.json"), []byte(authJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "history.jsonl"), []byte("native history"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.toml"), []byte("model = \"source-model\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "packages", "standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "packages", "standalone", "codex"), []byte("installer-owned"), 0o700); err != nil {
		t.Fatal(err)
	}
	process := codex.NewCodexProcess("codex", codex.Terminal{})
	importer := authusecase.NewImportCurrent(accounts, process, source)
	var output bytes.Buffer
	app := New(nil, importer, accounts, nil, nil, nil, &output)

	if err := app.Run(context.Background(), []string{"profile", "import-current", "main"}); err != nil {
		t.Fatal(err)
	}
	current, err := accounts.Current(context.Background())
	if err != nil || current.Name != "main" {
		t.Fatalf("active imported profile = %#v err=%v", current, err)
	}
	home := accounts.CodexHome(current.ID)
	for relative, expected := range map[string]string{
		"history.jsonl": "native history",
		"config.toml":   "model = \"source-model\"\n",
	} {
		content, err := os.ReadFile(filepath.Join(home, relative))
		if err != nil || string(content) != expected {
			t.Fatalf("imported %s = %q err=%v", relative, content, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, "packages")); !os.IsNotExist(err) {
		t.Fatalf("installer-owned root packages copied: %v", err)
	}
	if !strings.Contains(output.String(), "Imported current Codex login as main") {
		t.Fatalf("import-current output = %q", output.String())
	}
}

func TestHelpDocumentsImportCurrentInsecureFlag(t *testing.T) {
	var output bytes.Buffer
	if err := New(nil, nil, nil, nil, nil, nil, &output).Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "godex profile import-current [name] [--insecure]") {
		t.Fatalf("import-current help missing --insecure: %q", output.String())
	}
}

func TestDispatcherUsesActiveStandaloneProfileHome(t *testing.T) {
	root := t.TempDir()
	accounts := accountrepo.NewFileStore(root)
	profiles := profilerepo.NewStore(root)
	catalog := profileusecase.NewCatalog(profiles, accounts, t.TempDir())
	report, err := catalog.Add(context.Background(), profilemodel.AddRequest{Name: "standalone"})
	if err != nil {
		t.Fatal(err)
	}
	process := &dispatcherProcess{}
	runner := runtimeusecase.NewRunner(accounts, process, nil)
	app := New(nil, nil, accounts, runner, nil, nil, &bytes.Buffer{})
	app.SetProfiles(catalog)
	if err := app.Run(context.Background(), []string{"features", "list"}); err != nil {
		t.Fatal(err)
	}
	if process.home != report.Profile.CodexHome {
		t.Fatalf("launch home = %q, want %q", process.home, report.Profile.CodexHome)
	}
	if strings.Join(process.arguments, " ") != "features list" {
		t.Fatalf("arguments = %#v", process.arguments)
	}
}

func TestUpdateNoticeEligibilityMatchesReadOnlyAndMinimalSurfaces(t *testing.T) {
	for _, arguments := range [][]string{
		nil,
		{"run"},
		{"login", "--with-antigravity"},
		{"quota", "--once"},
		{"redeem", "work"},
		{"status", "--once"},
		{"doctor"},
	} {
		if !shouldShowUpdateNotice(arguments) {
			t.Fatalf("arguments %#v should show update notice", arguments)
		}
	}
	for _, arguments := range [][]string{
		{"info"},
		{"log", "last"},
		{"ping", "openai"},
		{"update"},
		{"quota", "--raw", "work"},
		{"doctor", "--json"},
		{"doctor", "--bundle", "out.json"},
		{"run", "--provider", "gemini", "--cli", "agy", "exec"},
		{"run", "session-id", "--provider", "gemini", "--cli", "agy"},
		{"run", "--provider", "gemini", "--cli", "agy", "--dry-run"},
		{"run", "--provider", "gemini", "--cli", "agy", "resume", "thread"},
		{"help"},
		{"--version"},
	} {
		if shouldShowUpdateNotice(arguments) {
			t.Fatalf("arguments %#v unexpectedly show update notice", arguments)
		}
	}
}

func TestNativeAntigravityDryRunSkipsUpdateNoticeLookup(t *testing.T) {
	store := updaterepo.NewStore(t.TempDir())
	if err := store.SaveLatest("9.9.9", time.Now()); err != nil {
		t.Fatal(err)
	}
	updater := updateusecase.NewUpdater(dispatcherReleaseSource{}, store, updategateway.NewInstaller(), "1.0.0")
	var output, notices bytes.Buffer
	agy := &dispatcherAntigravityProcess{}
	runner := runtimeusecase.NewRunner(nil, nil, nil)
	runner.SetAntigravityProcess(agy)
	runner.SetAntigravityCodexHome(t.TempDir())
	app := New(nil, nil, nil, runner, nil, nil, &output)
	app.SetUpdate(updater, &notices)
	if err := app.Run(context.Background(), []string{
		"run", "session-id", "--provider", "gemini", "--cli", "agy", "--dry-run",
	}); err != nil {
		t.Fatal(err)
	}
	if notices.Len() != 0 {
		t.Fatalf("native Antigravity emitted update notice: %q", notices.String())
	}
	if !strings.Contains(output.String(), "Provider: antigravity") {
		t.Fatalf("native Antigravity dry-run output = %q", output.String())
	}
	if agy.preparedHome == "" || len(agy.arguments) != 0 {
		t.Fatalf("dry-run prepared %q and launched arguments %#v", agy.preparedHome, agy.arguments)
	}
}

type dispatcherReleaseSource struct{}

func (dispatcherReleaseSource) LatestVersion(context.Context) (string, error) {
	return "9.9.9", nil
}

const (
	menuLoginNameOption = "--name"
	menuClaudeProfile   = "claude-menu"
	menuCopilotProfile  = "copilot-menu"
	menuAPIKeyProfile   = "api-key-menu"
)

type dispatcherClaudeSource struct {
	credential profilemodel.BuiltinCredential
}

func (source dispatcherClaudeSource) Load(context.Context) (profilemodel.BuiltinCredential, error) {
	return source.credential, nil
}

func (source dispatcherClaudeSource) InspectCredential(context.Context, string) (profilemodel.BuiltinCredential, error) {
	return source.credential, nil
}

type dispatcherCopilotSource struct {
	credential profilemodel.BuiltinCredential
}

type dispatcherAntigravityProcess struct {
	home         string
	preparedHome string
	arguments    []string
	err          error
}

func TestHelpDocumentsNativeAntigravityRuntimeAndLogin(t *testing.T) {
	var output bytes.Buffer
	if err := printHelp(&output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--provider gemini --cli agy", "--with-antigravity"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help missing %q", expected)
		}
	}
}

func (process *dispatcherAntigravityProcess) RunWithCodexHome(_ context.Context, home string, arguments []string) error {
	process.home = home
	process.arguments = append([]string(nil), arguments...)
	return process.err
}

func (process *dispatcherAntigravityProcess) RunRuntimeWithCodexHome(_ context.Context, home string, arguments []string) error {
	process.home = home
	process.arguments = append([]string(nil), arguments...)
	return process.err
}

func (process *dispatcherAntigravityProcess) PrepareCodexHome(home string) error {
	process.preparedHome = home
	return nil
}

func (source dispatcherCopilotSource) Load(context.Context) (profilemodel.BuiltinCredential, error) {
	return source.credential, nil
}

func TestDispatcherLoginMenuActionsUseExistingAuthAndProfileFlows(t *testing.T) {
	root := t.TempDir()
	accounts := accountrepo.NewFileStore(root)
	profiles := profilerepo.NewStore(root)
	catalog := profileusecase.NewCatalog(profiles, accounts, t.TempDir())
	claudeAccount, claudeMethod := "person@example.test", "claude-ai-oauth:pro"
	catalog.SetClaudeSource(dispatcherClaudeSource{credential: profilemodel.BuiltinCredential{
		Provider:    profilemodel.ProviderSnapshot{Kind: "anthropic", Account: &claudeAccount, AuthMethod: &claudeMethod},
		Email:       claudeAccount,
		SecretFiles: []profilemodel.ExportedSecretFile{{Path: ".credentials.json", Text: `{"accessToken":"fixture"}`}},
	}})
	copilotHost, copilotLogin := "https://github.example.test", "octocat"
	catalog.SetCopilotSource(dispatcherCopilotSource{credential: profilemodel.BuiltinCredential{
		Provider: profilemodel.ProviderSnapshot{Kind: "copilot", Host: &copilotHost, Login: &copilotLogin},
		Email:    copilotLogin,
	}})
	login := authusecase.NewLogin(dispatcherLoginAccounts{}, dispatcherLoginCodex{})
	agy := &dispatcherAntigravityProcess{}
	var output bytes.Buffer
	app := New(login, nil, accounts, nil, nil, nil, &output)
	app.SetProfiles(catalog)
	nativeAuth := authusecase.NewNative(nil, nil, agy)
	nativeAuth.SetAntigravityCodexHome(t.TempDir())
	nativeAuth.SetAntigravitySessionLocker(codex.SessionLocker{})
	app.SetNativeAuth(nativeAuth)
	app.SetInput(strings.NewReader("fixture-menu-api-key\n"))

	if err := app.runLoginMenuAction(context.Background(), authcli.LoginChatGPT, []string{menuLoginNameOption, "chatgpt-menu"}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginDeviceCode, []string{menuLoginNameOption, "device-menu"}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginOpenAIAPIKey, []string{menuLoginNameOption, menuAPIKeyProfile, "--base-url", "http://127.0.0.1:11434/v1"}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginClaude, []string{menuLoginNameOption, menuClaudeProfile}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginCopilotImport, []string{menuLoginNameOption, menuCopilotProfile}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginAntigravity, nil); err != nil {
		t.Fatal(err)
	}
	if err := app.runLogin(context.Background(), []string{"--with-antigravity"}); err != nil {
		t.Fatal(err)
	}
	listed, err := catalog.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, report := range listed {
		found[report.Profile.Name] = true
	}
	for _, name := range []string{menuAPIKeyProfile, menuClaudeProfile, menuCopilotProfile} {
		if !found[name] {
			t.Fatalf("login menu profile %q missing from %#v", name, found)
		}
	}
	if !strings.Contains(output.String(), "Logged in as chatgpt-menu") || !strings.Contains(output.String(), "Logged in as device-menu") || !strings.Contains(output.String(), "API-key profile \""+menuAPIKeyProfile+"\"") || !strings.Contains(output.String(), "profile \""+menuClaudeProfile+"\"") || !strings.Contains(output.String(), "profile \""+menuCopilotProfile+"\"") {
		t.Fatalf("login menu output = %q", output.String())
	}
	if strings.Contains(output.String(), "fixture-menu-api-key") {
		t.Fatalf("API key leaked into dispatcher output: %q", output.String())
	}
	if got := strings.Join(agy.arguments, " "); got != "auth login" {
		t.Fatalf("Antigravity login arguments = %#v", agy.arguments)
	}
	apiBaseURL, foundBaseURL, err := profiles.ReadOpenAICompatibleBaseURL(profiles.ManagedHome(menuAPIKeyProfile))
	if err != nil || !foundBaseURL || apiBaseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("menu API-key base URL = %q found=%t err=%v", apiBaseURL, foundBaseURL, err)
	}
}

func TestDispatcherLoginMenuUnsupportedMethodFailsExplicitly(t *testing.T) {
	app := New(nil, nil, nil, nil, nil, nil, &bytes.Buffer{})
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginGeminiAPIKeyGuidance, nil); err == nil || !strings.Contains(err.Error(), "guidance-only") {
		t.Fatalf("unsupported login action error = %v", err)
	}
}

func TestDispatcherDirectAPIKeyLoginCreatesOpenAICompatibleProfile(t *testing.T) {
	root := t.TempDir()
	accounts := accountrepo.NewFileStore(root)
	profiles := profilerepo.NewStore(root)
	catalog := profileusecase.NewCatalog(profiles, accounts, t.TempDir())
	var output bytes.Buffer
	app := New(nil, nil, accounts, nil, nil, nil, &output)
	app.SetProfiles(catalog)
	app.SetInput(strings.NewReader("fixture-direct-api-key\n"))
	err := app.runLogin(context.Background(), []string{"--with-api-key", "--name", "direct-api-key", "--openai-base-url", "https://example.test/v1"})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := profiles.Resolve(context.Background(), "direct-api-key")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Provider.Kind != profileentity.ProviderOpenAI || !profile.Managed {
		t.Fatalf("profile = %#v", profile)
	}
	baseURL, found, err := profiles.ReadOpenAICompatibleBaseURL(profile.CodexHome)
	if err != nil || !found || baseURL != "https://example.test/v1" {
		t.Fatalf("base URL = %q found=%t err=%v", baseURL, found, err)
	}
	if strings.Contains(output.String(), "fixture-direct-api-key") {
		t.Fatalf("API key leaked into direct login output: %q", output.String())
	}
}
