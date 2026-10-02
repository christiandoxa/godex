package auth

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	authusecase "github.com/christiandoxa/godex/internal/usecase/auth"
)

type fakeLoginAccounts struct {
	staged string
	device string
}

func (accounts *fakeLoginAccounts) CreateStagedHome() (string, error) {
	accounts.staged = "/synthetic/staged"
	return accounts.staged, nil
}

func (accounts *fakeLoginAccounts) RemoveStagedHome(path string) error {
	if path != accounts.staged {
		return errors.New("unexpected staged home")
	}
	return nil
}

func (accounts *fakeLoginAccounts) CommitLogin(_ context.Context, candidate accountentity.Account, _ string, _ bool) (accountentity.Account, error) {
	return candidate, nil
}

type fakeLoginCodex struct{}

func (fakeLoginCodex) Login(_ context.Context, _ string, deviceAuth bool) (accountentity.Identity, error) {
	if deviceAuth {
		return accountentity.Identity{Email: "device@example.com", ChatGPTAccountID: "device-account"}, nil
	}
	return accountentity.Identity{ChatGPTAccountID: "browser-account"}, nil
}

func TestLoginParsesFlagsAndRendersIdentity(t *testing.T) {
	accounts := &fakeLoginAccounts{}
	login := authusecase.NewLogin(accounts, fakeLoginCodex{})
	var output strings.Builder
	if err := Login(context.Background(), login, &output, []string{"--device-auth", "--name", "device"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Logged in as device (device@example.com).") {
		t.Fatalf("login output = %q", output.String())
	}
	output.Reset()
	if err := Login(context.Background(), login, &output, []string{"--name", "browser"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "Logged in as browser (") || !strings.HasSuffix(output.String(), ").\n") {
		t.Fatalf("fallback identity output = %q", output.String())
	}
}

func TestLoginRejectsUnexpectedArgumentsAndOutputErrors(t *testing.T) {
	login := authusecase.NewLogin(&fakeLoginAccounts{}, fakeLoginCodex{})
	if err := Login(context.Background(), login, io.Discard, []string{"positional"}); err == nil {
		t.Fatal("positional argument unexpectedly accepted")
	}
	if err := Login(context.Background(), login, failingWriter{}, nil); err == nil {
		t.Fatal("output failure unexpectedly ignored")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("synthetic output failure") }

func TestLoginMenuPromptEligibilityPreservesExplicitAndInvalidCLI(t *testing.T) {
	for _, arguments := range [][]string{nil, {"--name", "work"}, {"--name=work"}} {
		if !ShouldPromptLoginMenu(arguments) {
			t.Fatalf("arguments %#v should allow interactive menu", arguments)
		}
	}
	for _, arguments := range [][]string{{"--device-auth"}, {"positional"}, {"--unknown"}, {"--name"}} {
		if ShouldPromptLoginMenu(arguments) {
			t.Fatalf("arguments %#v unexpectedly allow interactive menu", arguments)
		}
	}
}

func TestParseLoginOptionsIsSharedByMenuAndDirectLogin(t *testing.T) {
	options, err := ParseLoginOptions([]string{"--name", "work", "--device-auth"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Name != "work" || !options.DeviceAuth {
		t.Fatalf("options = %#v", options)
	}
}
