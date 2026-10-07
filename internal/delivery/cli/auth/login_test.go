package auth

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	"github.com/christiandoxa/godex/internal/gateway/codex"
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

type fakeAntigravityProcess struct {
	home      string
	arguments []string
	err       error
}

func (process *fakeAntigravityProcess) RunWithCodexHome(_ context.Context, home string, arguments []string) error {
	process.home = home
	process.arguments = append([]string(nil), arguments...)
	return process.err
}

func TestLoginParsesFlagsAndRendersIdentity(t *testing.T) {
	accounts := &fakeLoginAccounts{}
	login := authusecase.NewLogin(accounts, fakeLoginCodex{})
	var output strings.Builder
	if err := Login(context.Background(), login, nil, &output, []string{"--device-auth", "--name", "device"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Logged in as device (device@example.com).") {
		t.Fatalf("login output = %q", output.String())
	}
	output.Reset()
	if err := Login(context.Background(), login, nil, &output, []string{"--name", "browser"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "Logged in as browser (") || !strings.HasSuffix(output.String(), ").\n") {
		t.Fatalf("fallback identity output = %q", output.String())
	}
}

func TestLoginRejectsUnexpectedArgumentsAndOutputErrors(t *testing.T) {
	login := authusecase.NewLogin(&fakeLoginAccounts{}, fakeLoginCodex{})
	if err := Login(context.Background(), login, nil, io.Discard, []string{"one", "two"}); err == nil {
		t.Fatal("multiple positional profiles unexpectedly accepted")
	}
	if err := Login(context.Background(), login, nil, failingWriter{}, nil); err == nil {
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
	for _, arguments := range [][]string{{"--device-auth"}, {"one", "two"}, {"--unknown"}, {"--name"}} {
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

func TestProdex04356LoginProfileSelectorsAndStatusMatchTaggedCLI(t *testing.T) {
	tests := []struct {
		arguments []string
		profile   string
		status    bool
		device    bool
	}{
		{[]string{"--profile", "work", "--device-auth"}, "work", false, true},
		{[]string{"-p", "work", "status"}, "work", true, false},
		{[]string{"work", "status"}, "work", true, false},
		{[]string{"work", "--device-auth"}, "work", false, true},
	}
	for _, test := range tests {
		options, err := ParseLoginOptions(test.arguments)
		if err != nil {
			t.Fatalf("%#v: %v", test.arguments, err)
		}
		if options.Profile != test.profile || options.Status != test.status || options.DeviceAuth != test.device {
			t.Fatalf("%#v => %#v", test.arguments, options)
		}
	}
	if _, err := ParseLoginOptions([]string{"--profile", "work", "--name", "other"}); err == nil {
		t.Fatal("--profile with --name unexpectedly accepted")
	}
	if _, err := ParseLoginOptions([]string{"work", "--profile", "other"}); err == nil {
		t.Fatal("positional profile with --profile unexpectedly accepted")
	}
	if !ShouldPromptLoginMenu([]string{"--profile", "work"}) {
		t.Fatal("selected profile without explicit method should retain interactive provider chooser")
	}
}

func TestProdex04356LoginBaseURLEmptyIsExplicitClear(t *testing.T) {
	for _, arguments := range [][]string{
		{"--with-api-key", "--base-url="},
		{"--with-api-key", "--base-url", ""},
		{"--with-api-key", "--openai-base-url="},
	} {
		options, err := ParseLoginOptions(arguments)
		if err != nil {
			t.Fatalf("%#v: %v", arguments, err)
		}
		if !options.WithAPIKey || !options.BaseURLSpecified || options.BaseURL != "" {
			t.Fatalf("%#v => %#v", arguments, options)
		}
	}
}

func TestParseLoginOptionsSupportsAPIKeyAndBaseURLAliases(t *testing.T) {
	options, err := ParseLoginOptions([]string{"--with-api-key", "--name", "work", "--base-url", "https://example.test/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.WithAPIKey || options.Name != "work" || !options.BaseURLSpecified || options.BaseURL != "https://example.test/v1" {
		t.Fatalf("options = %#v", options)
	}
	alias, err := ParseLoginOptions([]string{"--with-api-key", "--openai-base-url=http://localhost:11434/v1"})
	if err != nil || !alias.BaseURLSpecified || alias.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("alias = %#v, err=%v", alias, err)
	}
}

func TestLoginPromptEligibilityMatchesAPIKeyReference(t *testing.T) {
	if ShouldPromptLoginMenu([]string{"--with-api-key"}) {
		t.Fatal("explicit API-key login unexpectedly opened provider menu")
	}
	if !ShouldPromptLoginMenu([]string{"--base-url", "https://example.test/v1"}) {
		t.Fatal("base URL alone should still allow provider chooser")
	}
	login := authusecase.NewLogin(&fakeLoginAccounts{}, fakeLoginCodex{})
	if err := Login(context.Background(), login, nil, io.Discard, []string{"--base-url", "https://example.test/v1"}); err == nil || !strings.Contains(err.Error(), "only supported for API key") {
		t.Fatalf("base URL without API-key login error = %v", err)
	}
}

func TestAntigravityLoginDispatchAndOptionValidation(t *testing.T) {
	options, err := ParseLoginOptions([]string{"--with-antigravity"})
	if err != nil || !options.WithAntigravity || ShouldPromptLoginMenu([]string{"--with-antigravity"}) {
		t.Fatalf("Antigravity options = %#v, err=%v", options, err)
	}
	for _, arguments := range [][]string{
		{"--with-antigravity", "--name", "work"},
		{"--with-antigravity", "--base-url", "https://example.test"},
	} {
		if _, err := ParseLoginOptions(arguments); err == nil {
			t.Fatalf("Antigravity login accepted unsupported options %#v", arguments)
		}
	}
	devicePrecedence, err := ParseLoginOptions([]string{"--with-antigravity", "--device-auth"})
	if err != nil || !devicePrecedence.WithAntigravity {
		t.Fatalf("Antigravity/device precedence = %#v, err=%v", devicePrecedence, err)
	}
	apiPrecedence, err := ParseLoginOptions([]string{
		"--with-antigravity", "--with-api-key", "--name", "api-profile", "--base-url", "https://example.test/v1",
	})
	if err != nil || !apiPrecedence.WithAPIKey || apiPrecedence.WithAntigravity {
		t.Fatalf("API-key/Antigravity precedence = %#v, err=%v", apiPrecedence, err)
	}
	for _, alias := range []string{"--antigravity", "--with-agy", "--agy"} {
		options, err := ParseLoginOptions([]string{alias})
		if err != nil || !options.WithAntigravity {
			t.Fatalf("Antigravity alias %q = %#v, err=%v", alias, options, err)
		}
	}
	process := &fakeAntigravityProcess{}
	native := authusecase.NewNative(nil, nil, process)
	home := t.TempDir()
	native.SetAntigravityCodexHome(home)
	native.SetAntigravitySessionLocker(codex.SessionLocker{})
	if err := Login(context.Background(), nil, native, io.Discard, []string{"--with-antigravity"}); err != nil {
		t.Fatal(err)
	}
	if process.home != home || strings.Join(process.arguments, " ") != "auth login" {
		t.Fatalf("Antigravity login home/args = %q / %#v", process.home, process.arguments)
	}
}

func TestProdex04356LogoutSelectorsMatchProfileCLI(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"work"}, "work"},
		{[]string{"-p", "work"}, "work"},
		{[]string{"--profile", "work"}, "work"},
		{[]string{"--account", "work"}, "work"},
	} {
		got, err := ParseLogoutSelector(test.args)
		if err != nil || got != test.want {
			t.Fatalf("%#v => %q, err=%v", test.args, got, err)
		}
	}
	for _, args := range [][]string{
		{"one", "two"},
		{"-p", "one", "--profile", "two"},
		{"--unknown"},
	} {
		if _, err := ParseLogoutSelector(args); err == nil {
			t.Fatalf("logout selector %#v unexpectedly accepted", args)
		}
	}
}
