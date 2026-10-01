package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

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
		{"login", "--name", "work"}, {}, {"doctor"},
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
