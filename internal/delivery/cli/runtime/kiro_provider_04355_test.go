package runtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type kiroShortcutProfiles struct {
	target   profilemodel.LaunchTarget
	acquired []string
	released int
}

func (profiles *kiroShortcutProfiles) ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error) {
	return profilemodel.LaunchTarget{}, errors.New("unexpected ResolveLaunch")
}
func (profiles *kiroShortcutProfiles) ActiveLaunch(context.Context) (profilemodel.LaunchTarget, bool, error) {
	return profilemodel.LaunchTarget{}, false, nil
}
func (profiles *kiroShortcutProfiles) AcquireLaunch(_ context.Context, name string) (func() error, error) {
	profiles.acquired = append(profiles.acquired, name)
	return func() error { profiles.released++; return nil }, nil
}
func (profiles *kiroShortcutProfiles) ProviderLaunchPool(context.Context, string, string, bool) ([]profilemodel.LaunchTarget, error) {
	return nil, errors.New("Kiro provider shortcut must not rotate provider profiles")
}
func (profiles *kiroShortcutProfiles) ResolveProviderLaunch(_ context.Context, provider, requested string) (profilemodel.LaunchTarget, bool, error) {
	if provider != "kiro" || requested != "" {
		return profilemodel.LaunchTarget{}, false, errors.New("unexpected Kiro provider resolution")
	}
	return profiles.target, true, nil
}
func (profiles *kiroShortcutProfiles) AcquireLaunchPool(context.Context, []string) (func() error, error) {
	return nil, errors.New("Kiro provider shortcut must not acquire a pool")
}
func (profiles *kiroShortcutProfiles) OpenAICompatibleBaseURL(context.Context, string) (string, bool, error) {
	return "", false, nil
}

type kiroShortcutProcess struct {
	home, endpoint, provider string
	arguments                []string
}

func (*kiroShortcutProcess) Run(context.Context, string, []string) error {
	return errors.New("proxy path expected")
}
func (*kiroShortcutProcess) CheckProxySupport(context.Context) error { return nil }
func (process *kiroShortcutProcess) RunThroughProxy(_ context.Context, home, endpoint string, args []string) error {
	process.home, process.endpoint = home, endpoint
	process.arguments = append([]string(nil), args...)
	return nil
}
func (process *kiroShortcutProcess) RunThroughProxyProvider(_ context.Context, home, endpoint string, args []string, provider string) error {
	process.home, process.endpoint, process.provider = home, endpoint, provider
	process.arguments = append([]string(nil), args...)
	return nil
}

type kiroShortcutProxy struct{}

func (*kiroShortcutProxy) Start() error                { return nil }
func (*kiroShortcutProxy) Endpoint() string            { return "http://127.0.0.1:4567" }
func (*kiroShortcutProxy) Close(context.Context) error { return nil }

func TestProdex04355KiroProviderShortcutUsesImportedProfileWithoutAPIKey(t *testing.T) {
	home := t.TempDir()
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

	err := RunProfiles(t.Context(), runner, nil, profiles, []string{
		"--provider", "kiro", "--model", "kiro-model", "exec", "review",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if process.home != home || process.provider != "kiro" || process.endpoint != "http://127.0.0.1:4567" {
		t.Fatalf("Kiro launch = home %q provider %q endpoint %q", process.home, process.provider, process.endpoint)
	}
	if len(profiles.acquired) != 1 || profiles.acquired[0] != "kiro-main" || profiles.released != 1 {
		t.Fatalf("Kiro lease = %v released=%d", profiles.acquired, profiles.released)
	}
	if captured.Provider.Kind != "kiro" || captured.Provider.Name != "kiro-main" ||
		captured.Provider.DefaultModel != "kiro-model" || captured.Provider.APIURL != "https://kiro.dev" {
		t.Fatalf("Kiro provider config = %#v", captured.Provider)
	}
	accounts, err := captured.Accounts(t.Context())
	if err != nil || len(accounts) != 1 || accounts[0].Home != home || accounts[0].Provider.Kind != "kiro" {
		t.Fatalf("Kiro runtime accounts = %#v err=%v", accounts, err)
	}
}

func TestProdex04355KiroProviderShortcutRejectsNonKiroFallbackProfile(t *testing.T) {
	profiles := &kiroShortcutProfiles{target: profilemodel.LaunchTarget{
		Name: "openai-main", CodexHome: t.TempDir(), Provider: "openai",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "openai"},
	}}
	runner := runtimeusecase.NewRunner(nil, &kiroShortcutProcess{}, nil)
	err := RunProfiles(t.Context(), runner, nil, profiles, []string{"--provider", "kiro"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires an imported Kiro profile") {
		t.Fatalf("Kiro missing-profile error = %v", err)
	}
}
