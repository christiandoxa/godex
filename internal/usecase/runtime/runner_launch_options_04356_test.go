package runtime

import (
	"context"
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
