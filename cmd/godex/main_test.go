package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/config"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	updaterepo "github.com/christiandoxa/godex/internal/repository/update"
)

const antigravityLoginCaptureEnv = "GODEX_ANTIGRAVITY_LOGIN_CAPTURE"

func TestMain(m *testing.M) {
	if capturePath := os.Getenv(antigravityLoginCaptureEnv); capturePath != "" {
		captured, err := json.Marshal(struct {
			Arguments []string `json:"arguments"`
			CodexHome string   `json:"codex_home"`
		}{Arguments: os.Args[1:], CodexHome: os.Getenv(config.CodexHomeEnv)})
		if err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(capturePath, captured, 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(23)
	}
	os.Exit(m.Run())
}

func TestExitCodePreservesChildStatus(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestExitCodeChild$")
	command.Env = append(os.Environ(), "GODEX_EXIT_CODE_HELPER=1")
	err := command.Run()
	if got := exitCode(context.Background(), err); got != 23 {
		t.Fatalf("exit code = %d, want 23", got)
	}
}

func TestAntigravityExitCodeReportsChildFailureAndPreservesStatus(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestExitCodeChild$")
	command.Env = append(os.Environ(), "GODEX_EXIT_CODE_HELPER=1")
	err := command.Run()
	var stderr bytes.Buffer
	if got := antigravityExitCode(context.Background(), err, &stderr); got != 23 {
		t.Fatalf("Antigravity exit code = %d, want 23", got)
	}
	if got, want := stderr.String(), "Error: Antigravity CLI exited unsuccessfully\n"; got != want {
		t.Fatalf("Antigravity child diagnostic = %q, want %q", got, want)
	}
}

func TestRunAntigravityLoginUsesNormalStartupAndSharedHome(t *testing.T) {
	godexHome := t.TempDir()
	sharedHome := filepath.Join(t.TempDir(), "shared-codex")
	capturePath := filepath.Join(t.TempDir(), "agy-login.json")
	t.Setenv(config.HomeEnv, godexHome)
	t.Setenv(config.CodexHomeEnv, t.TempDir())
	t.Setenv(config.UpstreamEnv, "")
	t.Setenv(config.ProdexHomeEnv, t.TempDir())
	t.Setenv(config.ProdexSharedCodexHomeEnv, sharedHome)
	t.Setenv(config.AgyBinEnv, os.Args[0])
	t.Setenv(antigravityLoginCaptureEnv, capturePath)
	if err := updaterepo.NewStore(godexHome).SaveLatest("1.0.0", time.Now()); err != nil {
		t.Fatal(err)
	}
	original := os.Args
	os.Args = []string{"godex", "login", "--with-antigravity"}
	t.Cleanup(func() { os.Args = original })
	if got := run(); got != 23 {
		t.Fatalf("Antigravity login exit code = %d, want 23", got)
	}
	content, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	var captured struct {
		Arguments []string `json:"arguments"`
		CodexHome string   `json:"codex_home"`
	}
	if err := json.Unmarshal(content, &captured); err != nil {
		t.Fatal(err)
	}
	if got, want := captured.Arguments, []string{"auth", "login"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Antigravity login arguments = %#v, want %#v", got, want)
	}
	if captured.CodexHome != sharedHome {
		t.Fatalf("Antigravity login CODEX_HOME = %q, want %q", captured.CodexHome, sharedHome)
	}
}

func TestExitCodeReturnsCancelStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := exitCode(ctx, context.Canceled); got != 130 {
		t.Fatalf("exit code = %d, want 130", got)
	}
}

func TestRunListsAccountsFromConfiguredHome(t *testing.T) {
	t.Setenv("GODEX_HOME", t.TempDir())
	t.Setenv("GODEX_CODEX_BIN", "/synthetic/codex")
	original := os.Args
	os.Args = []string{"godex", "accounts"}
	t.Cleanup(func() { os.Args = original })
	if got := run(); got != 0 {
		t.Fatalf("run exit code = %d", got)
	}
}

func TestRunMapsUnknownCommandToFailure(t *testing.T) {
	t.Setenv("GODEX_HOME", t.TempDir())
	original := os.Args
	os.Args = []string{"godex", "synthetic-unknown"}
	t.Cleanup(func() { os.Args = original })
	if got := run(); got != 1 {
		t.Fatalf("run exit code = %d", got)
	}
}

func TestRunNativeAntigravityUsesMinimalStartup(t *testing.T) {
	t.Setenv(config.HomeEnv, t.TempDir())
	t.Setenv(config.CodexHomeEnv, string(os.PathSeparator))
	t.Setenv(config.UpstreamEnv, "not-a-valid-upstream")
	t.Setenv(config.ProdexHomeEnv, t.TempDir())
	sharedHome := filepath.Join(t.TempDir(), "shared-codex")
	t.Setenv(config.ProdexSharedCodexHomeEnv, sharedHome)
	t.Setenv(config.AgyBinEnv, "/missing/agy")
	original := os.Args
	os.Args = []string{
		"godex", "s", "gemini", "--no-presidio", "--no-sub-agent", "--cli", "agy",
		"--model", "gpt-6-luna", "--dry-run", "-c", "model_provider=\"openai\"", "exec",
	}
	t.Cleanup(func() { os.Args = original })
	if got := run(); got != 0 {
		t.Fatalf("minimal Antigravity dry-run exit code = %d", got)
	}
	info, err := os.Stat(sharedHome)
	if err != nil || !info.IsDir() {
		t.Fatalf("dry-run shared CODEX_HOME info = %#v, err=%v", info, err)
	}
}

func TestNativeAntigravityArgumentsIgnoresExplicitGodexCommands(t *testing.T) {
	for _, command := range []string{"doctor", "login", "quota", "profile", "help"} {
		if _, ok := nativeAntigravityArguments([]string{
			command, "--provider", "gemini", "--cli", "agy",
		}); ok {
			t.Fatalf("Antigravity intercepted explicit %s command", command)
		}
	}
}

func TestNativeAntigravityArgumentsAcceptsProdexSuperSyntax(t *testing.T) {
	for _, input := range [][]string{
		{"s", "gemini", "--cli", "agy"},
		{"super", "gemini", "--cli", "agy"},
		{"s", "--provider", "gemini", "--cli", "agy"},
		{"s", "--no-presidio", "--no-sub-agent", "gemini", "--cli", "agy"},
	} {
		arguments, ok := nativeAntigravityArguments(input)
		if !ok {
			t.Fatalf("native Antigravity did not recognize Prodex syntax %#v", input)
		}
		want := []string{"--provider", "gemini", "--cli", "agy"}
		if input[1] == "--provider" {
			want = input[1:]
		} else if input[1] == "--no-presidio" {
			want = []string{"--no-presidio", "--no-sub-agent", "--provider", "gemini", "--cli", "agy"}
		}
		if got, want := arguments, want; !reflect.DeepEqual(got, want) {
			t.Fatalf("normalized Prodex syntax = %#v, want %#v", got, want)
		}
	}
	if _, ok := nativeAntigravityArguments([]string{"s", "gemini"}); ok {
		t.Fatal("Super syntax without native Antigravity was intercepted")
	}
}

func TestExitCodeChild(t *testing.T) {
	if os.Getenv("GODEX_EXIT_CODE_HELPER") != "1" {
		return
	}
	os.Exit(23)
}

func TestRuntimeProviderHomePrefersSelectedAccount(t *testing.T) {
	config := proxyconfig.Config{
		PreferredAccount: "selected",
		Accounts: func(context.Context) ([]proxyconfig.Account, error) {
			return []proxyconfig.Account{
				{ID: "other", Home: "/profiles/other", Enabled: true},
				{ID: "selected", Home: "/profiles/selected", Enabled: true},
			}, nil
		},
	}
	home, err := runtimeProviderHome(config)
	if err != nil || home != "/profiles/selected" {
		t.Fatalf("home = %q, err = %v", home, err)
	}
}

func TestRuntimeProviderHomeUsesSingleProfileFallbackAndRejectsAmbiguity(t *testing.T) {
	config := proxyconfig.Config{
		Accounts: func(context.Context) ([]proxyconfig.Account, error) {
			return []proxyconfig.Account{{ID: "only", Home: "/profiles/only", Enabled: true}}, nil
		},
	}
	home, err := runtimeProviderHome(config)
	if err != nil || home != "/profiles/only" {
		t.Fatalf("single home = %q, err = %v", home, err)
	}
	config.Accounts = func(context.Context) ([]proxyconfig.Account, error) {
		return []proxyconfig.Account{
			{ID: "one", Home: "/profiles/one", Enabled: true},
			{ID: "two", Home: "/profiles/two", Enabled: true},
		}, nil
	}
	if _, err := runtimeProviderHome(config); err == nil {
		t.Fatal("ambiguous provider pool unexpectedly resolved a home")
	}
}

type fakeRuntimeAvailability struct {
	available map[string]bool
}

func (fake fakeRuntimeAvailability) Execute(context.Context, proxyconfig.Request, proxyconfig.Account) (*proxyconfig.Response, error) {
	return nil, nil
}

func (fake fakeRuntimeAvailability) AvailableAccount(id string) bool { return fake.available[id] }

func TestRuntimeAccountSourceFiltersUnavailableProviderProfiles(t *testing.T) {
	source := func(context.Context) ([]proxyconfig.Account, error) {
		return []proxyconfig.Account{
			{ID: "ready", Home: "/profiles/ready", Enabled: true},
			{ID: "broken", Home: "/profiles/broken", Enabled: true},
		}, nil
	}
	filtered := runtimeAccountSource(source, fakeRuntimeAvailability{available: map[string]bool{"ready": true}})
	accounts, err := filtered(context.Background())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "ready" {
		t.Fatalf("filtered accounts = %#v, err = %v", accounts, err)
	}
}

func TestNewRuntimeGatewayBuildsDeepSeekCredentialPool(t *testing.T) {
	gateway, err := newRuntimeGateway(proxyconfig.Config{
		Provider: proxyconfig.Provider{Kind: "deepseek", APIURL: "https://api.deepseek.com"},
		ProviderCredentials: []proxyconfig.ProviderCredential{
			{ID: "key-a", Secret: "secret-a"},
			{ID: "key-b", Secret: "secret-b"},
		},
	}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	available, ok := gateway.(runtimeAccountAvailability)
	if !ok || !available.AvailableAccount("key-a") || !available.AvailableAccount("key-b") || available.AvailableAccount("missing") {
		t.Fatalf("DeepSeek gateway availability = %#v / %t", gateway, ok)
	}
	if closer, ok := gateway.(interface{ Close() }); ok {
		closer.Close()
	}
}
