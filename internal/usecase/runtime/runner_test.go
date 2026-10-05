package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
	runtimerepo "github.com/christiandoxa/godex/internal/repository/runtime"
)

type fakeLaunchAccounts struct {
	accounts  []accountentity.Account
	homes     map[string]string
	listCalls int
	selectErr error
	listErr   error
}

func (fake *fakeLaunchAccounts) LaunchCandidates(_ context.Context, selector string) ([]accountentity.Account, error) {
	if fake.selectErr != nil {
		return nil, fake.selectErr
	}
	if selector != "" {
		for _, account := range fake.accounts {
			if account.Name == selector || account.ID == selector {
				return []accountentity.Account{account}, nil
			}
		}
		return nil, errors.New("not found")
	}
	return append([]accountentity.Account(nil), fake.accounts...), nil
}

func (fake *fakeLaunchAccounts) SelectForLaunch(_ context.Context, selector string) (accountentity.Account, error) {
	if fake.selectErr != nil {
		return accountentity.Account{}, fake.selectErr
	}
	if selector != "" {
		for _, account := range fake.accounts {
			if account.Name == selector || account.ID == selector {
				return account, nil
			}
		}
	}
	return fake.accounts[0], nil
}

func (fake *fakeLaunchAccounts) List(context.Context) ([]accountentity.Account, error) {
	fake.listCalls++
	if fake.listErr != nil {
		return nil, fake.listErr
	}
	return fake.accounts, nil
}

func (fake *fakeLaunchAccounts) CodexHome(id string) string {
	return fake.homes[id]
}

type fakeProcess struct {
	homes []string
	args  [][]string
}

func (fake *fakeProcess) Run(_ context.Context, home string, arguments []string) error {
	fake.homes = append(fake.homes, home)
	fake.args = append(fake.args, append([]string(nil), arguments...))
	return nil
}

func TestRunUsesSelectedProfilesAndPreservesArguments(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "two", Name: "aaa", Enabled: true, CreatedAt: time.Unix(20, 0)},
			{ID: "one", Name: "zzz", Enabled: true, CreatedAt: time.Unix(10, 0)},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	process := &fakeProcess{}
	runner := NewRunner(accounts, process, nil)
	runner.SetUpstreamURL("")

	if err := runner.Run(context.Background(), "one", []string{"--model", "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Run(context.Background(), "two", nil); err != nil {
		t.Fatal(err)
	}
	if len(process.homes) != 2 || process.homes[0] != "/profiles/one" || process.homes[1] != "/profiles/two" {
		t.Fatalf("run homes = %#v", process.homes)
	}
	if len(process.args[0]) != 2 || process.args[0][0] != "--model" {
		t.Fatalf("run args = %#v", process.args)
	}
}

type fakeProxyProcess struct {
	checked   bool
	home      string
	endpoint  string
	arguments []string
	checkErr  error
	runErr    error
}

func (fake *fakeProxyProcess) Run(context.Context, string, []string) error { return nil }

func (fake *fakeProxyProcess) CheckProxySupport(context.Context) error {
	fake.checked = true
	return fake.checkErr
}

func (fake *fakeProxyProcess) RunThroughProxy(_ context.Context, home, endpoint string, arguments []string) error {
	fake.home = home
	fake.endpoint = endpoint
	fake.arguments = append([]string(nil), arguments...)
	return fake.runErr
}

type fakeProxy struct {
	started      bool
	closed       bool
	closeContext context.Context
	closeError   error
	startError   error
}

func (fake *fakeProxy) Start() error {
	fake.started = true
	return fake.startError
}

func (fake *fakeProxy) Endpoint() string { return "http://127.0.0.1:1234" }

func (fake *fakeProxy) Close(ctx context.Context) error {
	fake.closed = true
	fake.closeContext = ctx
	return fake.closeError
}

type testContextKey struct{}

func TestRunBuildsProxyFromManagedAccountData(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:    map[string]string{"one": "/profiles/one"},
	}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetUpstreamURL("http://upstream.test/backend-api")

	requestContext := context.WithValue(context.Background(), testContextKey{}, "value")
	ctx, cancel := context.WithCancel(requestContext)
	cancel()
	if err := runner.Run(ctx, "one", []string{"--json"}); err != nil {
		t.Fatal(err)
	}
	if !process.checked || process.home != "/profiles/one" || process.endpoint != proxy.Endpoint() || len(process.arguments) != 1 || process.arguments[0] != "--json" {
		t.Fatalf("proxy process = %#v", process)
	}
	if !proxy.started || !proxy.closed || config.UpstreamURL != "http://upstream.test/backend-api" || config.PreferredAccount != "one" {
		t.Fatalf("proxy lifecycle/config = started:%t closed:%t config:%#v", proxy.started, proxy.closed, config)
	}
	if got := proxy.closeContext.Value(testContextKey{}); got != "value" {
		t.Fatalf("close context value = %v", got)
	}
	if proxy.closeError != nil {
		t.Fatalf("close context error = %v", proxy.closeError)
	}
	managed, err := config.Accounts(context.Background())
	if err != nil || len(managed) != 1 || managed[0].Home != "/profiles/one" || !managed[0].Enabled {
		t.Fatalf("managed accounts = %#v, err = %v", managed, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := config.Accounts(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled account source error = %v", err)
	}
}

func TestRunRequiresProxyFactoryAndPropagatesProxyErrors(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:    map[string]string{"one": "/profiles/one"},
	}
	if err := NewRunner(accounts, &fakeProxyProcess{}, nil).Run(context.Background(), "one", nil); err == nil {
		t.Fatal("missing proxy factory unexpectedly accepted")
	}
	checkErr := errors.New("synthetic proxy capability failure")
	process := &fakeProxyProcess{checkErr: checkErr}
	if err := NewRunner(accounts, process, func(proxyconfig.Config) (Proxy, error) {
		t.Fatal("proxy factory should not be called")
		return nil, nil
	}).Run(context.Background(), "one", nil); !errors.Is(err, checkErr) {
		t.Fatalf("capability error = %v", err)
	}
}

func TestRunReturnsChildAndProxyCloseErrors(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:    map[string]string{"one": "/profiles/one"},
	}
	childErr := errors.New("synthetic child failure")
	process := &fakeProxyProcess{runErr: childErr}
	proxy := &fakeProxy{}
	runner := NewRunner(accounts, process, func(proxyconfig.Config) (Proxy, error) { return proxy, nil })
	if err := runner.Run(context.Background(), "one", nil); !errors.Is(err, childErr) {
		t.Fatalf("child error = %v", err)
	}
	closeErr := errors.New("synthetic proxy close failure")
	process = &fakeProxyProcess{}
	proxy = &fakeProxy{closeError: closeErr}
	runner = NewRunner(accounts, process, func(proxyconfig.Config) (Proxy, error) { return proxy, nil })
	if err := runner.Run(context.Background(), "one", nil); !errors.Is(err, closeErr) {
		t.Fatalf("close error = %v", err)
	}
}

func TestRunPropagatesSelectionListingFactoryAndStartErrors(t *testing.T) {
	selectionErr := errors.New("synthetic selection failure")
	accounts := &fakeLaunchAccounts{selectErr: selectionErr}
	if err := NewRunner(accounts, &fakeProcess{}, nil).Run(context.Background(), "one", nil); !errors.Is(err, selectionErr) {
		t.Fatalf("selection error = %v", err)
	}
	listErr := errors.New("synthetic listing failure")
	accounts = &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:    map[string]string{"one": "/profiles/one"},
		listErr:  listErr,
	}
	if err := NewRunner(accounts, &fakeProxyProcess{}, func(proxyconfig.Config) (Proxy, error) {
		return &fakeProxy{}, nil
	}).Run(context.Background(), "one", nil); !errors.Is(err, listErr) {
		t.Fatalf("listing error = %v", err)
	}
	factoryErr := errors.New("synthetic factory failure")
	accounts.listErr = nil
	if err := NewRunner(accounts, &fakeProxyProcess{}, func(proxyconfig.Config) (Proxy, error) {
		return nil, factoryErr
	}).Run(context.Background(), "one", nil); !errors.Is(err, factoryErr) {
		t.Fatalf("factory error = %v", err)
	}
	startErr := errors.New("synthetic proxy start failure")
	if err := NewRunner(accounts, &fakeProxyProcess{}, func(proxyconfig.Config) (Proxy, error) {
		return &fakeProxy{startError: startErr}, nil
	}).Run(context.Background(), "one", nil); !errors.Is(err, startErr) {
		t.Fatalf("start error = %v", err)
	}
}

type fakeQuotaPreflight struct {
	ready map[string]bool
	errs  map[string]error
	calls []string
}

func (fake *fakeQuotaPreflight) Ready(_ context.Context, account accountentity.Account) (bool, error) {
	fake.calls = append(fake.calls, account.ID)
	if err := fake.errs[account.ID]; err != nil {
		return false, err
	}
	return fake.ready[account.ID], nil
}

func TestRunQuotaPreflightRotatesBeforeCommittingSelection(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	preflight := &fakeQuotaPreflight{ready: map[string]bool{"one": false, "two": true}, errs: map[string]error{}}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetQuotaPreflight(preflight)
	if err := runner.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if process.home != "/profiles/two" || config.PreferredAccount != "two" {
		t.Fatalf("selected home/preferred = %q / %q", process.home, config.PreferredAccount)
	}
	managed, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(managed) != 2 || !managed[0].Enabled || managed[0].EligibleAfter.IsZero() || !managed[1].EligibleAfter.IsZero() {
		t.Fatalf("quota-filtered accounts = %#v", managed)
	}
	if strings.Join(preflight.calls, ",") != "one,two" {
		t.Fatalf("quota preflight calls = %#v", preflight.calls)
	}
}

func TestRunQuotaPreflightFailsOpenOnProbeError(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	preflight := &fakeQuotaPreflight{
		ready: map[string]bool{"one": false},
		errs:  map[string]error{"two": errors.New("synthetic quota probe failure")},
	}
	process := &fakeProcess{}
	runner := NewRunner(accounts, process, nil)
	runner.SetQuotaPreflight(preflight)
	if err := runner.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if len(process.homes) != 1 || process.homes[0] != "/profiles/two" {
		t.Fatalf("fail-open homes = %#v", process.homes)
	}
}

func TestRunStopsBeforeLaunchWhenEveryEnabledAccountIsQuotaExhausted(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	preflight := &fakeQuotaPreflight{ready: map[string]bool{"one": false, "two": false}, errs: map[string]error{}}
	runner := NewRunner(accounts, &fakeProcess{}, nil)
	runner.SetQuotaPreflight(preflight)
	err := runner.Run(context.Background(), "", nil)
	if err == nil || !strings.Contains(err.Error(), "quota exhausted") {
		t.Fatalf("quota exhaustion error = %v", err)
	}
}

func TestRunExplicitQuotaExhaustedAccountDoesNotSilentlyRotate(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	preflight := &fakeQuotaPreflight{ready: map[string]bool{"one": false, "two": true}, errs: map[string]error{}}
	runner := NewRunner(accounts, &fakeProcess{}, nil)
	runner.SetQuotaPreflight(preflight)
	err := runner.Run(context.Background(), "one", nil)
	if err == nil || !strings.Contains(err.Error(), `account "one" is currently quota exhausted`) {
		t.Fatalf("explicit exhaustion error = %v", err)
	}
	if strings.Join(preflight.calls, ",") != "one" {
		t.Fatalf("explicit quota preflight calls = %#v", preflight.calls)
	}
}

func TestRunPreflightSnapshotsAccountsAfterFirstReady(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	preflight := &fakeQuotaPreflight{ready: map[string]bool{"one": true, "two": false}}
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetQuotaPreflight(preflight)
	if err := runner.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if process.home != "/profiles/one" || strings.Join(preflight.calls, ",") != "one,two" {
		t.Fatalf("selected home/calls = %q / %#v", process.home, preflight.calls)
	}
	managed, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(managed) != 2 || !managed[0].Enabled || managed[1].EligibleAfter.IsZero() {
		t.Fatalf("quota snapshot = %#v", managed)
	}
}

func TestExplicitAccountRestrictsEveryProxyRequest(t *testing.T) {
	accounts := &fakeLaunchAccounts{accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}, {ID: "two", Name: "two", Enabled: true}}, homes: map[string]string{"one": "/one", "two": "/two"}}
	var config proxyconfig.Config
	runner := NewRunner(accounts, &fakeProxyProcess{}, func(got proxyconfig.Config) (Proxy, error) { config = got; return &fakeProxy{}, nil })
	if err := runner.Run(context.Background(), "two", nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		profiles, err := config.Accounts(context.Background())
		if err != nil || len(profiles) != 1 || profiles[0].ID != "two" {
			t.Fatalf("fixed pool = %#v, %v", profiles, err)
		}
	}
}

func TestResumedSessionKeepsRolloutHomeAndRotatedUpstreamOwner(t *testing.T) {
	accounts := &fakeLaunchAccounts{accounts: []accountentity.Account{{ID: "home", Enabled: true}, {ID: "owner", Enabled: true}}, homes: map[string]string{"home": "/rollouts", "owner": "/credentials"}}
	process := &fakeProxyProcess{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) { config = got; return &fakeProxy{}, nil })
	runner.SetQuotaPreflight(&fakeQuotaPreflight{ready: map[string]bool{}})
	if err := runner.RunSession(context.Background(), "home", "owner", []string{"resume", "thread"}); err != nil {
		t.Fatal(err)
	}
	profiles, err := config.Accounts(context.Background())
	if err != nil || process.home != "/rollouts" || config.PreferredAccount != "owner" || len(profiles) != 1 || profiles[0].Home != "/credentials" {
		t.Fatalf("home/owner lost: %s, %v, %v", process.home, profiles, err)
	}
	if err := runner.RunSession(context.Background(), "home", "missing", nil); err == nil {
		t.Fatal("missing upstream owner rotated")
	}
}

func TestRunProfileBuildsSingleHomeProxyPool(t *testing.T) {
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(&fakeLaunchAccounts{}, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetUpstreamURL("http://upstream.test/backend-api")
	home := t.TempDir()
	if err := runner.RunProfile(context.Background(), home, []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if !process.checked || process.home != home || !proxy.started || !proxy.closed {
		t.Fatalf("profile proxy lifecycle = process:%#v proxy:%#v", process, proxy)
	}
	profiles, err := config.Accounts(context.Background())
	if err != nil || len(profiles) != 1 || profiles[0].Home != home || !profiles[0].Enabled {
		t.Fatalf("profile pool = %#v, err = %v", profiles, err)
	}
	if config.PreferredAccount == "" || config.PreferredAccount != profiles[0].ID || len(config.PreferredAccount) != 32 {
		t.Fatalf("profile routing id = %q, pool = %#v", config.PreferredAccount, profiles)
	}
}

func TestRunProviderProfilePropagatesCopilotConfigAndDefaults(t *testing.T) {
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(&fakeLaunchAccounts{}, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetProviderCatalogStore(runtimerepo.NewProviderCatalogStore())
	provider := CopilotProvider("copilot-work", "https://github.com", "octocat", "https://api.githubcopilot.com")
	home := t.TempDir()
	ctx := context.WithValue(context.Background(), testContextKey{}, "provider-context")
	if err := runner.RunProviderProfile(ctx, home, provider, []string{"--model", "custom-model", "exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if config.Provider != provider || config.Context.Value(testContextKey{}) != "provider-context" {
		t.Fatalf("provider/config context = %#v / %v", config.Provider, config.Context.Value(testContextKey{}))
	}
	if process.home != home || process.endpoint != proxy.Endpoint() || !proxy.started || !proxy.closed {
		t.Fatalf("provider proxy lifecycle = process:%#v proxy:%#v", process, proxy)
	}
	joined := strings.Join(process.arguments, " ")
	for _, want := range []string{
		"-c model_catalog_json=",
		`-c model="gpt-6-astra"`,
		"-c model_context_window=1050000",
		"-c model_auto_compact_token_limit=997500",
		"--model custom-model exec hello",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("provider arguments missing %q: %#v", want, process.arguments)
		}
	}
	accounts, err := config.Accounts(context.Background())
	if err != nil || len(accounts) != 1 || accounts[0].Home != home || !accounts[0].Enabled || accounts[0].ID == "" {
		t.Fatalf("provider accounts = %#v, err = %v", accounts, err)
	}
}

func TestProviderRuntimeArgumentsLeaveOpenAIUntouched(t *testing.T) {
	arguments := []string{"exec", "hello"}
	got := providerRuntimeArguments(proxyconfig.Provider{}, arguments)
	if strings.Join(got, " ") != "exec hello" {
		t.Fatalf("OpenAI arguments = %#v", got)
	}
}

func TestRunProviderProfilesBuildsRotatingProviderPool(t *testing.T) {
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(&fakeLaunchAccounts{}, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetProviderCatalogStore(runtimerepo.NewProviderCatalogStore())
	selectedHome, otherHome := t.TempDir(), t.TempDir()
	selected := CopilotProvider("copilot-a", "https://github-a.example.test", "alpha", "https://api-a.example.test")
	other := CopilotProvider("copilot-b", "https://github-b.example.test", "beta", "https://api-b.example.test")
	profiles := []proxyconfig.ProviderProfile{
		{Name: "copilot-a", Home: selectedHome, Provider: selected, Enabled: true},
		{Name: "copilot-b", Home: otherHome, Provider: other, Enabled: true},
	}
	if err := runner.RunProviderProfiles(context.Background(), selectedHome, selected, profiles, []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	accounts, err := config.Accounts(context.Background())
	if err != nil || len(accounts) != 2 {
		t.Fatalf("provider accounts = %#v, err = %v", accounts, err)
	}
	byHome := map[string]proxyconfig.Account{}
	for _, account := range accounts {
		byHome[account.Home] = account
	}
	if byHome[selectedHome].Provider != selected || byHome[otherHome].Provider != other {
		t.Fatalf("provider metadata lost: %#v", accounts)
	}
	if config.PreferredAccount == "" || config.PreferredAccount != byHome[selectedHome].ID {
		t.Fatalf("preferred account = %q, selected = %#v", config.PreferredAccount, byHome[selectedHome])
	}
	if process.home != selectedHome || !proxy.started || !proxy.closed {
		t.Fatalf("provider pool lifecycle = process:%#v proxy:%#v", process, proxy)
	}
}

func TestRunProviderAPIKeysBuildsSecretOnlySyntheticPool(t *testing.T) {
	process := &fakeProxyProcess{}
	proxy := &fakeProxy{}
	var config proxyconfig.Config
	runner := NewRunner(&fakeLaunchAccounts{}, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetProviderCatalogStore(runtimerepo.NewProviderCatalogStore())
	home := t.TempDir()
	runner.SetCurrentCodexHome(home)
	provider := AnthropicProvider("anthropic-api-key", "https://api.example.test/v1")
	keys := []string{"key with space", "second-key", "second-key"}
	if err := runner.RunProviderAPIKeys(context.Background(), "", provider, keys, []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	assertProviderAPIKeyPool(t, process, proxy, config, provider, home)
}

func assertProviderAPIKeyPool(
	t *testing.T,
	process *fakeProxyProcess,
	proxy *fakeProxy,
	config proxyconfig.Config,
	provider proxyconfig.Provider,
	home string,
) {
	t.Helper()
	if process.home != home || !proxy.started || !proxy.closed {
		t.Fatalf("synthetic key lifecycle = process:%#v proxy:%#v", process, proxy)
	}
	accounts, err := config.Accounts(context.Background())
	if err != nil || len(accounts) != 2 || len(config.ProviderCredentials) != 2 {
		t.Fatalf("synthetic pool = accounts:%#v credential_count:%d err:%v", accounts, len(config.ProviderCredentials), err)
	}
	assertProviderCredentialIdentities(t, accounts, config.ProviderCredentials, provider, home)
	if config.ProviderCredentials[0].Secret != "key with space" || config.ProviderCredentials[1].Secret != "second-key" {
		t.Fatal("provider credential order mismatch")
	}
	if config.PreferredAccount != config.ProviderCredentials[0].ID {
		t.Fatalf("preferred account = %q", config.PreferredAccount)
	}
	stable := providerCredentialRoutingID(provider, "second-key")
	if stable != config.ProviderCredentials[1].ID || stable != providerCredentialRoutingID(provider, "second-key") {
		t.Fatalf("provider key routing ID is unstable: %q", stable)
	}
}

func assertProviderCredentialIdentities(
	t *testing.T,
	accounts []proxyconfig.Account,
	credentials []proxyconfig.ProviderCredential,
	provider proxyconfig.Provider,
	home string,
) {
	t.Helper()
	for index, credential := range credentials {
		if credential.ID == "" || len(credential.ID) != 32 ||
			strings.Contains(credential.ID, "key") || credential.ID != accounts[index].ID {
			t.Fatalf("synthetic credential identity drifted: id=%q account=%#v", credential.ID, accounts[index])
		}
		if accounts[index].Provider != provider || accounts[index].Home != home || !accounts[index].Enabled {
			t.Fatalf("synthetic account = %#v", accounts[index])
		}
	}
}

type fakeProviderCredentialResolver struct {
	keys []string
	err  error
}

func (fake fakeProviderCredentialResolver) APIKeys(string, string) ([]string, error) {
	return append([]string(nil), fake.keys...), fake.err
}

func TestProviderAPIKeysUsesInjectedResolverAndRejectsUnsupportedProviders(t *testing.T) {
	runner := NewRunner(&fakeLaunchAccounts{}, &fakeProcess{}, nil)
	runner.SetProviderCredentialResolver(fakeProviderCredentialResolver{keys: []string{"one", "two"}})
	keys, err := runner.ProviderAPIKeys("anthropic", "ignored-by-fake")
	if err != nil || strings.Join(keys, ",") != "one,two" {
		t.Fatalf("resolved keys = %#v, err=%v", keys, err)
	}
	deepSeek, err := runner.ProviderAPIKeys("deepseek", "ignored-by-fake")
	if err != nil || strings.Join(deepSeek, ",") != "one,two" {
		t.Fatalf("DeepSeek resolved keys = %#v, err=%v", deepSeek, err)
	}
}

type fakeLeasedLaunchAccounts struct {
	fakeLaunchAccounts
	acquired []string
	released bool
}

func (fake *fakeLeasedLaunchAccounts) AcquireProfiles(_ context.Context, ids []string) (func() error, error) {
	fake.acquired = append([]string(nil), ids...)
	return func() error {
		fake.released = true
		return nil
	}, nil
}

func TestRunProviderAPIKeysAccountPinsManagedHome(t *testing.T) {
	home := t.TempDir()
	accounts := &fakeLeasedLaunchAccounts{fakeLaunchAccounts: fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "managed", Name: "managed", Enabled: true}},
		homes:    map[string]string{"managed": home},
	}}
	process := &fakeProcess{}
	runner := NewRunner(accounts, process, nil)
	provider := AnthropicProvider("raw-anthropic", "")
	if err := runner.RunProviderAPIKeysAccount(context.Background(), "managed", provider, []string{"fixture-key"}, []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if len(accounts.acquired) != 1 || accounts.acquired[0] != "managed" || !accounts.released {
		t.Fatalf("managed lease = acquired:%#v released:%t", accounts.acquired, accounts.released)
	}
	if len(process.homes) != 1 || process.homes[0] != home {
		t.Fatalf("provider raw-key home = %#v", process.homes)
	}
}

func TestRunPropagatesAutoRedeemToProxyConfig(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:    map[string]string{"one": "/profiles/one"},
	}
	process := &fakeProxyProcess{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return &fakeProxy{}, nil
	})
	runner.SetAutoRedeem(true)
	if err := runner.Run(context.Background(), "one", []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if !config.AutoRedeem {
		t.Fatal("auto-redeem flag was not propagated to proxy config")
	}

	config = proxyconfig.Config{}
	runner.SetAutoRedeem(false)
	if err := runner.Run(context.Background(), "one", []string{"exec", "hello"}); err != nil {
		t.Fatal(err)
	}
	if config.AutoRedeem {
		t.Fatal("auto-redeem unexpectedly remained enabled")
	}
}

func TestRunAutoRedeemLaunchesProxyWhenEveryAccountIsQuotaExhausted(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	preflight := &fakeQuotaPreflight{ready: map[string]bool{"one": false, "two": false}, errs: map[string]error{}}
	process := &fakeProxyProcess{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return &fakeProxy{}, nil
	})
	runner.SetQuotaPreflight(preflight)
	runner.SetAutoRedeem(true)
	if err := runner.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if process.home != "/profiles/one" || config.PreferredAccount != "one" || !config.AutoRedeem {
		t.Fatalf("auto-redeem launch home/config = %q / %#v", process.home, config)
	}
	managed, err := config.Accounts(context.Background())
	if err != nil || len(managed) != 2 || managed[0].EligibleAfter.IsZero() || managed[1].EligibleAfter.IsZero() {
		t.Fatalf("auto-redeem exhausted pool = %#v, err = %v", managed, err)
	}
}

func TestRunAutoRedeemAllowsExplicitExhaustedAccountIntoProxyOnly(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	preflight := &fakeQuotaPreflight{ready: map[string]bool{"one": false, "two": true}, errs: map[string]error{}}
	process := &fakeProxyProcess{}
	var config proxyconfig.Config
	runner := NewRunner(accounts, process, func(got proxyconfig.Config) (Proxy, error) {
		config = got
		return &fakeProxy{}, nil
	})
	runner.SetQuotaPreflight(preflight)
	runner.SetAutoRedeem(true)
	if err := runner.Run(context.Background(), "one", nil); err != nil {
		t.Fatal(err)
	}
	managed, err := config.Accounts(context.Background())
	if err != nil || len(managed) != 1 || managed[0].ID != "one" || managed[0].EligibleAfter.IsZero() {
		t.Fatalf("explicit auto-redeem pool = %#v, err = %v", managed, err)
	}
	if strings.Join(preflight.calls, ",") != "one" {
		t.Fatalf("explicit auto-redeem preflight calls = %#v", preflight.calls)
	}
}

type sharedSessionPreparingProcess struct {
	prepared []string
	shared   []string
	runHome  string
	runArgs  []string
}

func (process *sharedSessionPreparingProcess) PrepareSharedSessionHome(home, shared string) error {
	process.prepared = append(process.prepared, home)
	process.shared = append(process.shared, shared)
	return nil
}

func (process *sharedSessionPreparingProcess) Run(_ context.Context, home string, args []string) error {
	if len(process.prepared) != 2 {
		return errors.New("native child started before all managed homes were prepared")
	}
	process.runHome = home
	process.runArgs = append([]string(nil), args...)
	return nil
}

func TestRunPreparesAllManagedHomesForNativeSharedSessionPicker(t *testing.T) {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	process := &sharedSessionPreparingProcess{}
	runner := NewRunner(accounts, process, nil)
	runner.SetSharedCodexHome("/shared/codex")

	if err := runner.Run(t.Context(), "one", []string{"resume"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(process.prepared, []string{"/profiles/one", "/profiles/two"}) ||
		!reflect.DeepEqual(process.shared, []string{"/shared/codex", "/shared/codex"}) {
		t.Fatalf("shared preparation = homes %#v roots %#v", process.prepared, process.shared)
	}
	if process.runHome != "/profiles/one" || !reflect.DeepEqual(process.runArgs, []string{"resume"}) {
		t.Fatalf("native picker launch = home %q args %#v", process.runHome, process.runArgs)
	}
}
