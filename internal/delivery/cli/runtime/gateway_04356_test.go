package runtime

import (
	"bytes"
	"context"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type gatewayTestAccounts struct {
	account accountentity.Account
	home    string
}

func (fake gatewayTestAccounts) LaunchCandidates(context.Context, string) ([]accountentity.Account, error) {
	return []accountentity.Account{fake.account}, nil
}
func (fake gatewayTestAccounts) SelectForLaunch(context.Context, string) (accountentity.Account, error) {
	return fake.account, nil
}
func (fake gatewayTestAccounts) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{fake.account}, nil
}
func (fake gatewayTestAccounts) CodexHome(string) string { return fake.home }
func (fake gatewayTestAccounts) Current(context.Context) (accountentity.Account, error) {
	return fake.account, nil
}
func (fake gatewayTestAccounts) Resolve(context.Context, string) (accountentity.Account, error) {
	return fake.account, nil
}

type gatewayTestProcess struct{}

func (gatewayTestProcess) Run(context.Context, string, []string) error { return nil }

type gatewayTestProxy struct {
	started bool
	closed  bool
}

func (fake *gatewayTestProxy) Start() error {
	fake.started = true
	return nil
}
func (fake *gatewayTestProxy) Endpoint() string { return "http://127.0.0.1:4321" }
func (fake *gatewayTestProxy) Close(context.Context) error {
	fake.closed = true
	return nil
}

func TestProdex04356GatewayCLIUsesFixedOpenAIMountAndCloses(t *testing.T) {
	proxy := &gatewayTestProxy{}
	var config proxymodel.Config
	runner := runtimeusecase.NewRunner(
		gatewayTestAccounts{
			account: accountentity.Account{ID: "account-a", Name: "account-a", Enabled: true},
			home:    "/profiles/account-a",
		},
		gatewayTestProcess{},
		func(got proxymodel.Config) (runtimeusecase.Proxy, error) {
			config = got
			return proxy, nil
		},
	)
	runner.SetUpstreamURL("https://chatgpt.com/backend-api")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := Gateway(ctx, runner, nil, &out, []string{
		"--listen", "127.0.0.1:4321",
		"--base-url", "https://example.test/backend-api",
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Status: listening",
		"Endpoint: http://127.0.0.1:4321/backend-api/godex",
		"Provider: openai",
		"Stop: Ctrl-C",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("gateway output missing %q: %s", want, out.String())
		}
	}
	if !proxy.started || !proxy.closed || config.UpstreamURL != "https://example.test/backend-api" ||
		config.ListenAddr != "127.0.0.1:4321" || !config.SkipQuotaPreflight || config.AutoRedeem {
		t.Fatalf("gateway lifecycle/config = started:%t closed:%t config:%#v", proxy.started, proxy.closed, config)
	}
}

func TestProdex04356GatewayArgumentsMatchTaggedSurface(t *testing.T) {
	options, err := parseGatewayArguments([]string{
		"--listen=127.0.0.1:9000",
		"--provider", "claude",
		"--url", "https://api.example.test/v1",
		"--api-key", "synthetic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.listen != "127.0.0.1:9000" || options.provider != "anthropic" ||
		options.baseURL != "https://api.example.test/v1" || options.apiKey != "synthetic" {
		t.Fatalf("gateway options = %#v", options)
	}
	if _, err := parseGatewayArguments([]string{"--api-key", "synthetic"}); err == nil {
		t.Fatal("--api-key without provider unexpectedly accepted")
	}
	if _, err := parseGatewayArguments([]string{"--presidio", "--no-presidio"}); err == nil {
		t.Fatal("conflicting Presidio flags unexpectedly accepted")
	}
}

func TestProdex04356GatewaySmartContextAndPresidioReachRuntimeProxy(t *testing.T) {
	proxy := &gatewayTestProxy{}
	var captured proxymodel.Config
	var requiredValues []bool
	runner := runtimeusecase.NewRunner(
		gatewayTestAccounts{
			account: accountentity.Account{ID: "account-a", Name: "account-a", Enabled: true},
			home:    "/profiles/account-a",
		},
		gatewayTestProcess{},
		func(got proxymodel.Config) (runtimeusecase.Proxy, error) {
			captured = got
			return proxy, nil
		},
	)
	runner.SetUpstreamURL("https://chatgpt.com/backend-api")
	runner.SetPresidioConfigResolver(func(_ context.Context, required bool) (*proxymodel.PresidioConfig, error) {
		requiredValues = append(requiredValues, required)
		return &proxymodel.PresidioConfig{
			AnalyzerURL:   "http://127.0.0.1:5002",
			AnonymizerURL: "http://127.0.0.1:5001",
		}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := Gateway(ctx, runner, nil, &out, []string{
		"--smart-context", "--presidio",
	}); err != nil {
		t.Fatal(err)
	}
	if !captured.SmartContextEnabled || captured.Presidio == nil {
		t.Fatalf("gateway feature config = %#v", captured)
	}
	if len(requiredValues) != 1 || requiredValues[0] {
		t.Fatalf("gateway Presidio required calls = %#v", requiredValues)
	}

	captured = proxymodel.Config{}
	proxy.started, proxy.closed = false, false
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	out.Reset()
	if err := Gateway(ctx, runner, nil, &out, []string{
		"--smart-context", "--no-presidio",
	}); err != nil {
		t.Fatal(err)
	}
	if !captured.SmartContextEnabled || captured.Presidio != nil {
		t.Fatalf("gateway no-presidio config = %#v", captured)
	}
}
