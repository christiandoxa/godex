package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
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
