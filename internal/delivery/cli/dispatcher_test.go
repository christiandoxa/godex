package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	authcli "github.com/christiandoxa/godex/internal/delivery/cli/auth"
	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	accountrepo "github.com/christiandoxa/godex/internal/repository/account"
	profilerepo "github.com/christiandoxa/godex/internal/repository/profile"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
	profileusecase "github.com/christiandoxa/godex/internal/usecase/profile"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
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

type dispatcherLoginCodex struct{}

func (dispatcherLoginCodex) Login(context.Context, string, bool) (accountentity.Identity, error) {
	return accountentity.Identity{Email: "login@example.com", ChatGPTAccountID: "login-account"}, nil
}

func (dispatcherLoginCodex) ImportCurrent(context.Context, string, string) (accountentity.Identity, error) {
	return accountentity.Identity{Email: "<redacted>", ChatGPTAccountID: "<redacted>"}, nil
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
		{"help"},
		{"--version"},
	} {
		if shouldShowUpdateNotice(arguments) {
			t.Fatalf("arguments %#v unexpectedly show update notice", arguments)
		}
	}
}

const (
	menuLoginNameOption = "--name"
	menuClaudeProfile   = "claude-menu"
	menuCopilotProfile  = "copilot-menu"
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
	var output bytes.Buffer
	app := New(login, nil, accounts, nil, nil, nil, &output)
	app.SetProfiles(catalog)

	if err := app.runLoginMenuAction(context.Background(), authcli.LoginChatGPT, []string{menuLoginNameOption, "chatgpt-menu"}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginDeviceCode, []string{menuLoginNameOption, "device-menu"}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginClaude, []string{menuLoginNameOption, menuClaudeProfile}); err != nil {
		t.Fatal(err)
	}
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginCopilotImport, []string{menuLoginNameOption, menuCopilotProfile}); err != nil {
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
	for _, name := range []string{menuClaudeProfile, menuCopilotProfile} {
		if !found[name] {
			t.Fatalf("login menu profile %q missing from %#v", name, found)
		}
	}
	if !strings.Contains(output.String(), "Logged in as chatgpt-menu") || !strings.Contains(output.String(), "Logged in as device-menu") || !strings.Contains(output.String(), `profile "`+menuClaudeProfile+`"`) || !strings.Contains(output.String(), `profile "`+menuCopilotProfile+`"`) {
		t.Fatalf("login menu output = %q", output.String())
	}
}

func TestDispatcherLoginMenuUnsupportedMethodFailsExplicitly(t *testing.T) {
	app := New(nil, nil, nil, nil, nil, nil, &bytes.Buffer{})
	if err := app.runLoginMenuAction(context.Background(), authcli.LoginOpenAIAPIKey, nil); err == nil || !strings.Contains(err.Error(), "guidance-only") {
		t.Fatalf("unsupported login action error = %v", err)
	}
}
