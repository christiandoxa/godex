package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

type superCredentialResolver struct {
	keys map[string][]string
}

func (resolver superCredentialResolver) APIKeys(provider, explicit string) ([]string, error) {
	if explicit != "" {
		return []string{explicit}, nil
	}
	return append([]string(nil), resolver.keys[provider]...), nil
}

func TestProdex04356SuperLaunchKiroProfileUsesOverlaySmartContextAndLease(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"auto\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles := &kiroShortcutProfiles{target: profilemodel.LaunchTarget{
		Name: "kiro-main", CodexHome: home, Provider: "kiro",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "kiro"},
	}}
	process := &kiroShortcutProcess{}
	var captured proxyconfig.Config
	runner := runtimeusecase.NewRunner(nil, process, func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return &kiroShortcutProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))

	options, err := parseSuperArguments([]string{
		"--provider", "kiro", "--model", "kiro-model", "exec", "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := launchSuperProfiles(t.Context(), runner, nil, profiles, options); err != nil {
		t.Fatal(err)
	}
	if process.home == home || !strings.HasPrefix(filepath.Base(process.home), ".godex-overlay-") {
		t.Fatalf("Kiro Super child home = %q base=%q", process.home, home)
	}
	if process.provider != "kiro" {
		t.Fatalf("Kiro Super provider = %q", process.provider)
	}
	if !captured.SmartContextEnabled || !captured.SkipQuotaPreflight {
		t.Fatalf("Kiro Super proxy config = %#v", captured)
	}
	accounts, err := captured.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Home != home || accounts[0].Provider.Kind != "kiro" {
		t.Fatalf("Kiro Super routing accounts = %#v", accounts)
	}
	if len(profiles.acquired) != 1 || profiles.acquired[0] != "kiro-main" || profiles.released != 1 {
		t.Fatalf("Kiro Super lease = acquired:%#v released:%d", profiles.acquired, profiles.released)
	}
	if _, err := os.Stat(process.home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Kiro Super overlay survived exit: %v", err)
	}
}

type superProviderProfiles struct {
	home string
}

func (profiles *superProviderProfiles) ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error) {
	return profilemodel.LaunchTarget{}, errors.New("unexpected ResolveLaunch")
}
func (profiles *superProviderProfiles) ActiveLaunch(context.Context) (profilemodel.LaunchTarget, bool, error) {
	return profilemodel.LaunchTarget{}, false, nil
}
func (profiles *superProviderProfiles) AcquireLaunch(context.Context, string) (func() error, error) {
	return func() error { return nil }, nil
}
func (profiles *superProviderProfiles) ProviderLaunchPool(context.Context, string, string, bool) ([]profilemodel.LaunchTarget, error) {
	return nil, errors.New("unexpected ProviderLaunchPool")
}
func (profiles *superProviderProfiles) ResolveProviderLaunch(_ context.Context, provider, requested string) (profilemodel.LaunchTarget, bool, error) {
	if provider != "deepseek" || requested != "" {
		return profilemodel.LaunchTarget{}, false, errors.New("unexpected provider resolution")
	}
	return profilemodel.LaunchTarget{Name: "deepseek-home", CodexHome: profiles.home, Provider: "deepseek"}, true, nil
}
func (profiles *superProviderProfiles) AcquireLaunchPool(context.Context, []string) (func() error, error) {
	return nil, errors.New("unexpected AcquireLaunchPool")
}
func (profiles *superProviderProfiles) OpenAICompatibleBaseURL(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func TestProdex04356SuperLaunchDeepSeekAPIKeyFixedPoolUsesOverlay(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"deepseek\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles := &superProviderProfiles{home: home}
	process := &kiroShortcutProcess{}
	var captured proxyconfig.Config
	runner := runtimeusecase.NewRunner(nil, process, func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return &kiroShortcutProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))
	runner.SetProviderCredentialResolver(superCredentialResolver{
		keys: map[string][]string{"deepseek": {"key-a", "key-b"}},
	})

	options, err := parseSuperArguments([]string{
		"--provider", "deepseek", "--no-auto-rotate", "--model", "deepseek-chat",
		"exec", "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := launchSuperProfiles(t.Context(), runner, nil, profiles, options); err != nil {
		t.Fatal(err)
	}
	if process.home == home || !strings.HasPrefix(filepath.Base(process.home), ".godex-overlay-") {
		t.Fatalf("DeepSeek Super child home = %q base=%q", process.home, home)
	}
	if process.provider != "deepseek" {
		t.Fatalf("DeepSeek Super provider = %q", process.provider)
	}
	if captured.Provider.Kind != "deepseek" || captured.Provider.DefaultModel != "deepseek-chat" ||
		!captured.SmartContextEnabled || !captured.SkipQuotaPreflight {
		t.Fatalf("DeepSeek Super config = %#v", captured)
	}
	routed, err := captured.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || routed[0].ID != captured.PreferredAccount || routed[0].Home != home {
		t.Fatalf("DeepSeek fixed routing pool = preferred:%q accounts:%#v", captured.PreferredAccount, routed)
	}
	if len(captured.ProviderCredentials) != 2 {
		t.Fatalf("DeepSeek credential snapshot = %#v", captured.ProviderCredentials)
	}
}

func TestProdex04356SuperLocalRewriteRequiresResolvableBaseHome(t *testing.T) {
	runner := runtimeusecase.NewRunner(nil, &kiroShortcutProcess{}, nil)
	options, err := parseSuperArguments([]string{"--url", "http://127.0.0.1:11434/v1"})
	if err != nil {
		t.Fatal(err)
	}
	err = launchSuperProfiles(t.Context(), runner, nil, nil, options)
	if err == nil || !strings.Contains(err.Error(), "active account lookup is not configured") {
		t.Fatalf("local Super launch = %v", err)
	}
}

func TestProdex04356SuperNoProxyMapsToUpstreamPolicyWithoutDisablingRuntimeProxy(t *testing.T) {
	options, err := parseSuperArguments([]string{"--no-proxy"})
	if err != nil {
		t.Fatal(err)
	}
	launch := superRuntimeLaunchOptions(options)
	if !launch.UpstreamNoProxy || !launch.SmartContextEnabled || !launch.SuperOverlay {
		t.Fatalf("no-proxy Super launch options = %#v", launch)
	}
}

type superSessionAccounts struct {
	homes map[string]string
}

func (accounts superSessionAccounts) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{
		{ID: "home", Name: "home", Enabled: true},
		{ID: "owner", Name: "owner", Enabled: true},
	}, nil
}
func (accounts superSessionAccounts) CodexHome(id string) string { return accounts.homes[id] }
func (accounts superSessionAccounts) LaunchCandidates(context.Context, string) ([]accountentity.Account, error) {
	return nil, errors.New("fresh launch selection unexpectedly used")
}
func (accounts superSessionAccounts) SelectForLaunch(context.Context, string) (accountentity.Account, error) {
	return accountentity.Account{}, errors.New("fresh launch selection unexpectedly used")
}

type superSessionReader struct {
	home string
	id   string
}

func (reader superSessionReader) List(_ context.Context, home string) ([]sessionentity.Session, error) {
	if home != reader.home {
		return nil, nil
	}
	return []sessionentity.Session{{
		ID: reader.id, ThreadName: "Super Resume", Source: "cli",
		Path: filepath.Join(home, "sessions", reader.id+".jsonl"), UpdatedUnix: 10,
	}}, nil
}

func TestProdex04356SuperLaunchBareSessionPreservesOwnerAndInjectsFullAccessAfterResolution(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	owner := filepath.Join(root, "owner")
	for _, path := range []string{home, owner} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	accounts := superSessionAccounts{homes: map[string]string{"home": home, "owner": owner}}
	process := &kiroShortcutProcess{}
	var captured proxyconfig.Config
	runner := runtimeusecase.NewRunner(accounts, process, func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return &kiroShortcutProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(root, "profiles"))
	const sessionID = "00000000-0000-4000-8000-000000000123"
	catalog := sessionusecase.NewCatalog(accounts, superSessionReader{home: home, id: sessionID}, runner)
	catalog.SetOwnerLookup(func(context.Context, string) (string, error) { return "owner", nil })

	options, err := parseSuperArguments([]string{sessionID, "continue"})
	if err != nil {
		t.Fatal(err)
	}
	if err := launchSuperProfiles(t.Context(), runner, catalog, nil, options); err != nil {
		t.Fatal(err)
	}
	if captured.PreferredAccount != "owner" {
		t.Fatalf("Super session preferred owner = %q", captured.PreferredAccount)
	}
	routed, err := captured.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || routed[0].ID != "owner" || routed[0].Home != owner {
		t.Fatalf("Super session routed accounts = %#v", routed)
	}
	if process.home == home || !strings.HasPrefix(filepath.Base(process.home), ".godex-overlay-") {
		t.Fatalf("Super session child home = %q base=%q", process.home, home)
	}
	joined := strings.Join(process.arguments, "\n")
	for _, expected := range []string{
		"--dangerously-bypass-approvals-and-sandbox",
		"resume",
		sessionID,
		"continue",
		"features.apps=false",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("Super session args missing %q: %#v", expected, process.arguments)
		}
	}
	if !captured.SmartContextEnabled {
		t.Fatal("Super session lost Smart Context")
	}
}

type superSubAgentCaptureProcess struct {
	spec   map[string]any
	agents string
}

func (*superSubAgentCaptureProcess) Run(context.Context, string, []string) error {
	return errors.New("proxy path expected")
}

func (*superSubAgentCaptureProcess) CheckProxySupport(context.Context) error { return nil }

func (process *superSubAgentCaptureProcess) RunThroughProxy(_ context.Context, home, _ string, _ []string) error {
	return process.capture(home)
}

func (process *superSubAgentCaptureProcess) RunThroughProxyProvider(_ context.Context, home, _ string, _ []string, _ string) error {
	return process.capture(home)
}

func (process *superSubAgentCaptureProcess) capture(home string) error {
	content, err := os.ReadFile(filepath.Join(home, "sub-agent-launch.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, &process.spec); err != nil {
		return err
	}
	agents, err := os.ReadFile(filepath.Join(home, "AGENTS.md"))
	if err != nil {
		return err
	}
	process.agents = string(agents)
	return nil
}

func TestProdex04356SuperLaunchWritesInheritedSubAgentOverlayBeforeChild(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"auto\""), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles := &kiroShortcutProfiles{target: profilemodel.LaunchTarget{
		Name: "kiro-main", CodexHome: home, Provider: "kiro",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "kiro"},
	}}
	process := &superSubAgentCaptureProcess{}
	runner := runtimeusecase.NewRunner(nil, process, func(proxyconfig.Config) (runtimeusecase.Proxy, error) {
		return &kiroShortcutProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))

	options, err := parseSuperArguments([]string{
		"--provider", "kiro",
		"--sub-agent",
		"--sub-agent-provider", "deepseek",
		"--sub-agent-model", "deepseek-chat",
		"--sub-agent-model-reasoning-effort", "high",
		"--sub-agent-max-concurrency", "7",
		"--presidio",
		"--require-tool", "rtk",
		"exec", "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := launchSuperProfiles(t.Context(), runner, nil, profiles, options); err != nil {
		t.Fatal(err)
	}
	if process.spec["provider"] != "deepseek" ||
		process.spec["model"] != "deepseek-chat" ||
		process.spec["effort"] != "high" ||
		process.spec["presidio-enabled"] != true ||
		process.spec["recursion-marker"] != "GODEX_SUB_AGENT" {
		t.Fatalf("sub-agent launch spec = %#v", process.spec)
	}
	maximum, ok := process.spec["max-concurrency"].(map[string]any)
	if !ok || maximum["value"] != float64(7) || maximum["source"] != "custom" {
		t.Fatalf("sub-agent max concurrency = %#v", process.spec["max-concurrency"])
	}
	required, ok := process.spec["required-tools"].([]any)
	if !ok || len(required) != 1 || required[0] != "rtk" {
		t.Fatalf("sub-agent required tools = %#v", process.spec["required-tools"])
	}
	for _, want := range []string{
		"Inherited Presidio: enabled",
		"Inherited required tools: rtk",
		"Maximum active sub-agents: 7 (custom)",
	} {
		if !strings.Contains(process.agents, want) {
			t.Fatalf("sub-agent AGENTS block missing %q", want)
		}
	}
}

func TestProdex04356SuperRejectsExplicitSubAgentRecursion(t *testing.T) {
	t.Setenv("GODEX_SUB_AGENT", "1")
	if _, err := parseSuperArguments([]string{"--sub-agent"}); err == nil ||
		!strings.Contains(err.Error(), "cannot be re-enabled") {
		t.Fatalf("recursive sub-agent enable = %v", err)
	}
	options, err := parseSuperArguments([]string{"--no-sub-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if options.subAgent.enabled {
		t.Fatal("--no-sub-agent unexpectedly enabled recursion")
	}
}

type localSuperCaptureProcess struct {
	home, endpoint, provider string
	arguments                []string
	auth                     map[string]any
}

func (*localSuperCaptureProcess) Run(context.Context, string, []string) error {
	return errors.New("local Super must use runtime rewrite proxy")
}

func (*localSuperCaptureProcess) CheckProxySupport(context.Context) error { return nil }

func (process *localSuperCaptureProcess) RunThroughProxy(_ context.Context, home, endpoint string, arguments []string) error {
	return process.capture(home, endpoint, arguments, "")
}

func (process *localSuperCaptureProcess) RunThroughProxyProvider(
	_ context.Context,
	home, endpoint string,
	arguments []string,
	provider string,
) error {
	return process.capture(home, endpoint, arguments, provider)
}

func (process *localSuperCaptureProcess) capture(home, endpoint string, arguments []string, provider string) error {
	process.home, process.endpoint, process.provider = home, endpoint, provider
	process.arguments = append([]string(nil), arguments...)
	content, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		return err
	}
	return json.Unmarshal(content, &process.auth)
}

func TestProdex04356SuperLocalRewriteUsesOverlayProxyAndSyntheticAuth(t *testing.T) {
	baseHome := t.TempDir()
	baseAuth := []byte("{\"auth_mode\":\"apikey\",\"OPENAI_API_KEY\":\"profile-secret\"}\n")
	if err := os.WriteFile(filepath.Join(baseHome, "auth.json"), baseAuth, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "config.toml"), []byte("model = \"base\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	process := &localSuperCaptureProcess{}
	var captured proxyconfig.Config
	runner := runtimeusecase.NewRunner(nil, process, func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return &kiroShortcutProxy{}, nil
	})
	runner.SetCurrentCodexHome(baseHome)
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))

	options, err := parseSuperArguments([]string{
		"--url", "http://127.0.0.1:11434",
		"--model", "qwen-local",
		"--context-window", "262144",
		"--auto-compact-token-limit", "240000",
		"--no-proxy",
		"exec", "review",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := launchSuperProfiles(t.Context(), runner, nil, nil, options); err != nil {
		t.Fatal(err)
	}

	if captured.Provider.Kind != "local" ||
		captured.Provider.APIURL != "http://127.0.0.1:11434/v1" ||
		captured.Provider.DefaultModel != "qwen-local" ||
		captured.Provider.ContextWindow != 262144 ||
		captured.Provider.AutoCompactLimit != 240000 {
		t.Fatalf("local provider config = %#v", captured.Provider)
	}
	if !captured.SmartContextEnabled || !captured.SkipQuotaPreflight || !captured.UpstreamNoProxy {
		t.Fatalf("local proxy policy = %#v", captured)
	}
	accounts, err := captured.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].Provider.Kind != "local" ||
		accounts[0].Home != baseHome {
		t.Fatalf("local routing accounts = %#v err=%v", accounts, err)
	}

	if process.home == baseHome || process.endpoint != "http://127.0.0.1:4567" || process.provider != "local" {
		t.Fatalf("local child = home:%q endpoint:%q provider:%q", process.home, process.endpoint, process.provider)
	}
	if process.auth["auth_mode"] != "apikey" ||
		process.auth["OPENAI_API_KEY"] != "godex-runtime-provider" {
		t.Fatalf("overlay auth = %#v", process.auth)
	}
	for _, want := range []string{
		"model=\"qwen-local\"",
		"model_context_window=262144",
		"model_auto_compact_token_limit=240000",
		"model_reasoning_summary=\"none\"",
		"web_search=\"disabled\"",
		"features.apps=false",
		"features.js_repl=false",
		"features.image_generation=false",
		"--dangerously-bypass-approvals-and-sandbox",
	} {
		if !slices.Contains(process.arguments, want) {
			t.Fatalf("local child args missing %q: %#v", want, process.arguments)
		}
	}
	if _, err := os.Stat(process.home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local overlay survived child exit: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(baseHome, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(baseAuth) {
		t.Fatalf("base auth changed: %q", after)
	}
}

func TestProdex04356SuperLocalRewriteResumeKeepsSessionHomeAndSyntheticTransport(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	owner := filepath.Join(root, "owner")
	for _, path := range []string{home, owner} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "config.toml"), []byte("model = \"base\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "auth.json"), []byte("{\"auth_mode\":\"apikey\",\"OPENAI_API_KEY\":\"profile-secret\"}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	accounts := superSessionAccounts{homes: map[string]string{"home": home, "owner": owner}}
	process := &localSuperCaptureProcess{}
	var captured proxyconfig.Config
	runner := runtimeusecase.NewRunner(accounts, process, func(config proxyconfig.Config) (runtimeusecase.Proxy, error) {
		captured = config
		return &kiroShortcutProxy{}, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(root, "profiles"))
	const sessionID = "00000000-0000-4000-8000-000000000456"
	catalog := sessionusecase.NewCatalog(accounts, superSessionReader{home: home, id: sessionID}, runner)
	catalog.SetOwnerLookup(func(context.Context, string) (string, error) { return "owner", nil })

	options, err := parseSuperArguments([]string{
		"--url", "http://127.0.0.1:11434",
		"--model", "qwen-local",
		sessionID, "continue",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := launchSuperProfiles(t.Context(), runner, catalog, nil, options); err != nil {
		t.Fatal(err)
	}
	routed, err := captured.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if captured.Provider.Kind != "local" || len(routed) != 1 || routed[0].Home != home ||
		routed[0].Provider.Kind != "local" {
		t.Fatalf("local resume routing = provider:%#v accounts:%#v", captured.Provider, routed)
	}
	if process.provider != "local" || process.home == home {
		t.Fatalf("local resume child = provider:%q home:%q", process.provider, process.home)
	}
	if process.auth["OPENAI_API_KEY"] != "godex-runtime-provider" {
		t.Fatalf("local resume overlay auth = %#v", process.auth)
	}
	joined := strings.Join(process.arguments, "\n")
	for _, want := range []string{
		"--dangerously-bypass-approvals-and-sandbox",
		"resume",
		sessionID,
		"continue",
		"model=\"qwen-local\"",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("local resume args missing %q: %#v", want, process.arguments)
		}
	}
}
