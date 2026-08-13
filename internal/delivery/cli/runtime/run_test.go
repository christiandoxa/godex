package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type fakeRunnerAccounts struct {
	selected string
}

func (accounts *fakeRunnerAccounts) SelectForLaunch(_ context.Context, selector string) (accountentity.Account, error) {
	accounts.selected = selector
	return accountentity.Account{ID: "synthetic-account", Enabled: true}, nil
}

func (fakeRunnerAccounts) List(context.Context) ([]accountentity.Account, error) { return nil, nil }

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
	if err := Run(context.Background(), runner, []string{"--account", "work", "--", "--model", "synthetic"}); err != nil {
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

func TestDoctorRendersReport(t *testing.T) {
	doctor := runtimeusecase.NewDoctor(fakeDoctorAccounts{}, fakeVersionedCodex{})
	var output strings.Builder
	if err := Doctor(context.Background(), doctor, &output, nil); err != nil {
		t.Fatal(err)
	}
	want := "Godex home: /synthetic/godex\nCodex: codex synthetic\nAccounts: 2 (1 enabled)\n"
	if output.String() != want {
		t.Fatalf("doctor output = %q", output.String())
	}
	if err := Doctor(context.Background(), doctor, &output, []string{"extra"}); err == nil {
		t.Fatal("doctor accepted an argument")
	}
	if err := Doctor(context.Background(), doctor, failingWriter{}, nil); err == nil {
		t.Fatal("doctor output failure was ignored")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic output failure") }
