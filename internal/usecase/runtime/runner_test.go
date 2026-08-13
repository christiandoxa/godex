package runtime

import (
	"context"
	"errors"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type fakeLaunchAccounts struct {
	accounts  []accountentity.Account
	homes     map[string]string
	listCalls int
	selectErr error
	listErr   error
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
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
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
	if accounts.listCalls != 1 {
		t.Fatalf("account list calls = %d", accounts.listCalls)
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
