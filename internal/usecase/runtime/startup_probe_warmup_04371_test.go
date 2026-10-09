package runtime

import (
	"context"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

type startupWarmupQuota04371 struct {
	events   *[]string
	accounts []accountentity.Account
}

func (quota *startupWarmupQuota04371) Ready(context.Context, accountentity.Account) (bool, error) {
	return true, nil
}

func (quota *startupWarmupQuota04371) WarmupStartupProbes(
	_ context.Context,
	accounts []accountentity.Account,
	_ string,
	_ bool,
) {
	*quota.events = append(*quota.events, "warmup")
	quota.accounts = append([]accountentity.Account(nil), accounts...)
}

type startupWarmupProcess04371 struct{ events *[]string }

func (process *startupWarmupProcess04371) Run(context.Context, string, []string) error {
	*process.events = append(*process.events, "native")
	return nil
}

func (process *startupWarmupProcess04371) CheckProxySupport(context.Context) error {
	return nil
}

func (process *startupWarmupProcess04371) RunThroughProxy(context.Context, string, string, []string) error {
	*process.events = append(*process.events, "child")
	return nil
}

type startupWarmupProxy04371 struct{ events *[]string }

func (proxy *startupWarmupProxy04371) Start() error {
	*proxy.events = append(*proxy.events, "proxy")
	return nil
}

func (*startupWarmupProxy04371) Endpoint() string            { return "http://127.0.0.1:1" }
func (*startupWarmupProxy04371) Close(context.Context) error { return nil }

func TestProdex04371StartupWarmupRunsAfterProxyStartBeforeChildOnSkip(t *testing.T) {
	events := []string{}
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	quota := &startupWarmupQuota04371{events: &events}
	process := &startupWarmupProcess04371{events: &events}
	runner := NewRunner(accounts, process, func(proxyconfig.Config) (Proxy, error) {
		return &startupWarmupProxy04371{events: &events}, nil
	})
	runner.SetQuotaPreflight(quota)

	if err := runner.RunWithOptions(t.Context(), "", nil, RuntimeLaunchOptions{
		SkipQuotaPreflight: true,
		UpstreamURL:        "https://quota.example/backend-api",
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"proxy", "warmup", "child"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("startup event order = %#v, want %#v", got, want)
	}
	if len(quota.accounts) != 2 || quota.accounts[0].ID != "one" || quota.accounts[1].ID != "two" {
		t.Fatalf("warmup accounts = %#v", quota.accounts)
	}
}
