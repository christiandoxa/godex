package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04356RuntimeLaunchOptionsReachProxyWithoutMutatingDefaults(t *testing.T) {
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(nil, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetUpstreamURL("https://chatgpt.com/backend-api")
	runner.SetAutoRedeem(true)
	disabled := false

	err := runner.launchHomeWithOptions(
		context.Background(),
		"/profiles/one",
		"profile-one",
		proxyconfig.Provider{},
		nil,
		[]proxyconfig.Account{{ID: "profile-one", Home: "/profiles/one", Enabled: true}},
		[]string{"--json"},
		RuntimeLaunchOptions{
			SmartContextEnabled: true,
			SkipQuotaPreflight:  true,
			AutoRedeem:          &disabled,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !config.SmartContextEnabled || !config.SkipQuotaPreflight || config.AutoRedeem {
		t.Fatalf("launch options did not reach proxy config: %#v", config)
	}
	if !runner.autoRedeem {
		t.Fatal("per-launch auto-redeem override mutated runner default")
	}
	if process.home != "/profiles/one" || len(process.arguments) != 1 || process.arguments[0] != "--json" {
		t.Fatalf("child launch = home:%q args:%#v", process.home, process.arguments)
	}

	config = proxyconfig.Config{}
	proxy.started, proxy.closed = false, false
	if err := runner.launchHome(
		context.Background(),
		"/profiles/two",
		"profile-two",
		proxyconfig.Provider{},
		nil,
		[]proxyconfig.Account{{ID: "profile-two", Home: "/profiles/two", Enabled: true}},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if config.SmartContextEnabled || config.SkipQuotaPreflight || !config.AutoRedeem {
		t.Fatalf("normal runtime defaults changed: %#v", config)
	}
}

func TestProdex04356ManagedProfilesRootIsPerRunnerState(t *testing.T) {
	first := NewRunner(nil, &fakeProcess{}, nil)
	second := NewRunner(nil, &fakeProcess{}, nil)
	first.SetManagedProfilesRoot("/godex/profiles")
	if first.managedProfilesRoot != "/godex/profiles" {
		t.Fatalf("managed root = %q", first.managedProfilesRoot)
	}
	if second.managedProfilesRoot != "" {
		t.Fatalf("managed root leaked across runners: %q", second.managedProfilesRoot)
	}
}

func TestProdex04356RuntimeSuperOverlayIsChildOnlyAndCleansUp(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(t.TempDir(), "profiles")
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(nil, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetManagedProfilesRoot(managed)
	runner.SetUpstreamURL("https://chatgpt.com/backend-api")

	err := runner.launchHomeWithOptions(
		context.Background(),
		base,
		"profile-one",
		proxyconfig.Provider{},
		nil,
		[]proxyconfig.Account{{ID: "profile-one", Home: base, Enabled: true}},
		nil,
		RuntimeLaunchOptions{SuperOverlay: true, SmartContextEnabled: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if process.home == base || !strings.HasPrefix(filepath.Base(process.home), ".godex-overlay-") {
		t.Fatalf("Super child home = %q, base = %q", process.home, base)
	}
	if _, err := os.Stat(process.home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Super overlay survived child exit: %v", err)
	}
	accounts, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Home != base {
		t.Fatalf("router account home was overlaid: %#v", accounts)
	}
	if !config.SmartContextEnabled {
		t.Fatal("Smart Context was lost on Super overlay launch")
	}
	if got, err := os.ReadFile(filepath.Join(base, "config.toml")); err != nil || string(got) != "model = \"gpt-5.4\"\n" {
		t.Fatalf("base profile changed: %q err=%v", got, err)
	}
}

func TestProdex04356RuntimeSuperOverlayRequiresManagedRoot(t *testing.T) {
	runner := NewRunner(nil, &fakeProxyProcess{}, func(proxyconfig.Config) (Proxy, error) {
		return &fakeProxy{}, nil
	})
	err := runner.launchHomeWithOptions(
		context.Background(),
		"/profiles/one",
		"profile-one",
		proxyconfig.Provider{},
		nil,
		[]proxyconfig.Account{{ID: "profile-one", Home: "/profiles/one", Enabled: true}},
		nil,
		RuntimeLaunchOptions{SuperOverlay: true},
	)
	if err == nil || !strings.Contains(err.Error(), "managed profile root") {
		t.Fatalf("missing managed root = %v", err)
	}
}

func TestProdex04356RuntimeSuperSessionKeepsOwnerWhileOverlayingRolloutHome(t *testing.T) {
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
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "home", Name: "home", Enabled: true},
			{ID: "owner", Name: "owner", Enabled: true},
		},
		homes: map[string]string{"home": home, "owner": owner},
	}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(root, "profiles"))
	runner.SetUpstreamURL("https://chatgpt.com/backend-api")

	err := runner.RunSessionWithOptions(
		context.Background(), "home", "owner", []string{"resume", "thread-id"},
		RuntimeLaunchOptions{
			SuperOverlay: true, SmartContextEnabled: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.PreferredAccount != "owner" {
		t.Fatalf("session preferred owner = %q", config.PreferredAccount)
	}
	routed, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || routed[0].ID != "owner" || routed[0].Home != owner {
		t.Fatalf("session routed accounts = %#v", routed)
	}
	if process.home == home || !strings.HasPrefix(filepath.Base(process.home), ".godex-overlay-") {
		t.Fatalf("session child did not use rollout overlay: %q", process.home)
	}
}

func TestProdex04356RuntimeSuperProviderAPIKeysKeepsCredentialPool(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(nil, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetManagedProfilesRoot(filepath.Join(root, "profiles"))
	provider := DeepSeekProvider("deepseek-api-key", "")

	err := runner.RunProviderAPIKeysWithOptions(
		context.Background(), home, provider, []string{"key-a", "key-b"}, nil,
		RuntimeLaunchOptions{
			SuperOverlay: true, SmartContextEnabled: true, SkipQuotaPreflight: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.Provider.Kind != "deepseek" || len(config.ProviderCredentials) != 2 ||
		!config.SmartContextEnabled || !config.SkipQuotaPreflight {
		t.Fatalf("provider Super config = %#v", config)
	}
	routed, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 2 || routed[0].Home != home || routed[1].Home != home {
		t.Fatalf("provider routing homes = %#v", routed)
	}
	if process.home == home || !strings.HasPrefix(filepath.Base(process.home), ".godex-overlay-") {
		t.Fatalf("provider child did not use overlay: %q", process.home)
	}
}

func TestProdex04356RuntimeLaunchOptionsCanDisableAccountRotation(t *testing.T) {
	root := t.TempDir()
	homeOne := filepath.Join(root, "one")
	homeTwo := filepath.Join(root, "two")
	for _, home := range []string{homeOne, homeTwo} {
		if err := os.MkdirAll(home, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": homeOne, "two": homeTwo},
	}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	rotate := false
	if err := runner.RunWithOptions(
		context.Background(), "one", nil,
		RuntimeLaunchOptions{AllowAutoRotate: &rotate},
	); err != nil {
		t.Fatal(err)
	}
	routed, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || routed[0].ID != "one" {
		t.Fatalf("fixed launch pool = %#v", routed)
	}
}

func TestProdex04356RuntimeLaunchOptionsFixProviderCredentialPool(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(nil, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	provider := DeepSeekProvider("deepseek-api-key", "")
	rotate := false
	if err := runner.RunProviderAPIKeysWithOptions(
		context.Background(), home, provider, []string{"key-a", "key-b"}, nil,
		RuntimeLaunchOptions{AllowAutoRotate: &rotate},
	); err != nil {
		t.Fatal(err)
	}
	routed, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || routed[0].ID != config.PreferredAccount {
		t.Fatalf("fixed provider pool = preferred:%q accounts:%#v", config.PreferredAccount, routed)
	}
	if len(config.ProviderCredentials) != 2 {
		t.Fatalf("credential snapshot should stay complete for gateway setup: %#v", config.ProviderCredentials)
	}
}

type noProxyPolicyQuota struct {
	calls []bool
}

func (quota *noProxyPolicyQuota) Ready(context.Context, accountentity.Account) (bool, error) {
	return false, errors.New("legacy Ready unexpectedly used")
}

func (quota *noProxyPolicyQuota) AvailabilityWithPolicy(
	_ context.Context,
	_ accountentity.Account,
	noProxy bool,
) (quotamodel.Availability, error) {
	quota.calls = append(quota.calls, noProxy)
	return quotamodel.Availability{Ready: true}, nil
}

func TestProdex04356NoProxyPolicyReachesQuotaAndRuntimeProxy(t *testing.T) {
	home := t.TempDir()
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:    map[string]string{"one": home},
	}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	quota := &noProxyPolicyQuota{}
	runner.SetQuotaPreflight(quota)
	runner.SetManagedProfilesRoot(filepath.Join(t.TempDir(), "profiles"))

	if err := runner.RunWithOptions(
		t.Context(), "", nil,
		RuntimeLaunchOptions{UpstreamNoProxy: true},
	); err != nil {
		t.Fatal(err)
	}
	if len(quota.calls) != 1 || !quota.calls[0] {
		t.Fatalf("quota no-proxy calls = %#v", quota.calls)
	}
	if !config.UpstreamNoProxy {
		t.Fatalf("runtime proxy config lost upstream no-proxy: %#v", config)
	}
}

func TestProdex04356PresidioLaunchResolverReachesProxyAndRequiredPolicy(t *testing.T) {
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var captured proxyconfig.Config
	var requiredValues []bool
	runner := NewRunner(nil, process, func(got proxyconfig.Config) (Proxy, error) {
		captured = got
		return proxy, nil
	})
	runner.SetPresidioConfigResolver(func(_ context.Context, required bool) (*proxyconfig.PresidioConfig, error) {
		requiredValues = append(requiredValues, required)
		return &proxyconfig.PresidioConfig{
			AnalyzerURL:   "http://127.0.0.1:5002",
			AnonymizerURL: "http://127.0.0.1:5001",
			FailClosed:    required,
		}, nil
	})

	err := runner.launchHomeWithOptions(
		t.Context(), "/profiles/presidio", "profile-presidio",
		proxyconfig.Provider{}, nil,
		[]proxyconfig.Account{{ID: "profile-presidio", Home: "/profiles/presidio", Enabled: true}},
		nil,
		RuntimeLaunchOptions{PresidioEnabled: true, PresidioRequired: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(requiredValues) != 1 || !requiredValues[0] {
		t.Fatalf("required resolver calls = %#v", requiredValues)
	}
	if captured.Presidio == nil || !captured.Presidio.FailClosed ||
		captured.Presidio.AnalyzerURL != "http://127.0.0.1:5002" {
		t.Fatalf("presidio proxy config = %#v", captured.Presidio)
	}

	runner.SetPresidioConfigResolver(nil)
	err = runner.launchHomeWithOptions(
		t.Context(), "/profiles/presidio", "profile-presidio",
		proxyconfig.Provider{}, nil,
		[]proxyconfig.Account{{ID: "profile-presidio", Home: "/profiles/presidio", Enabled: true}},
		nil,
		RuntimeLaunchOptions{PresidioEnabled: true},
	)
	if err == nil || !strings.Contains(err.Error(), "resolver is not configured") {
		t.Fatalf("missing resolver = %v", err)
	}
}
