package runtime

import (
	"context"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04356GatewayAccountUsesFixedProfileWithoutQuotaPreflight(t *testing.T) {
	home := t.TempDir()
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Name: "one", Enabled: true}},
		homes:    map[string]string{"one": home},
	}
	proxy := &fakeProxy{}
	var config proxymodel.Config
	runner := NewRunner(accounts, &fakeProcess{}, func(got proxymodel.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetUpstreamURL("https://chatgpt.com/backend-api")

	gateway, err := runner.StartGatewayAccount(t.Context(), "one", GatewayStartOptions{ListenAddr: "127.0.0.1:4567", SmartContextEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !proxy.started {
		t.Fatal("gateway proxy was not started")
	}
	if config.ListenAddr != "127.0.0.1:4567" || !config.SkipQuotaPreflight || config.AutoRedeem ||
		config.PreferredAccount != "one" || config.Provider.Kind != "" || !config.SmartContextEnabled {
		t.Fatalf("gateway config = %#v", config)
	}
	got, err := config.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "one" || got[0].Home != home || !got[0].Enabled {
		t.Fatalf("gateway fixed pool = %#v", got)
	}
	if gateway.Endpoint() != "http://127.0.0.1:1234/backend-api/godex" {
		t.Fatalf("gateway endpoint = %q", gateway.Endpoint())
	}
	if err := gateway.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !proxy.closed {
		t.Fatal("gateway proxy was not closed")
	}
}

func TestProdex04356GatewayAPIKeysKeepsProviderCredentialPool(t *testing.T) {
	currentHome := t.TempDir()
	proxy := &fakeProxy{}
	var config proxymodel.Config
	runner := NewRunner(&fakeLaunchAccounts{}, &fakeProcess{}, func(got proxymodel.Config) (Proxy, error) {
		config = got
		return proxy, nil
	})
	runner.SetCurrentCodexHome(currentHome)
	provider := DeepSeekProvider("deepseek-api-key", "")

	gateway, err := runner.StartGatewayAPIKeys(
		context.Background(), "", provider, []string{"key-a", "key-b"}, GatewayStartOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close(context.Background())
	if !config.SkipQuotaPreflight || config.AutoRedeem || config.Provider.Kind != "deepseek" ||
		len(config.ProviderCredentials) != 2 {
		t.Fatalf("provider gateway config = %#v", config)
	}
	got, err := config.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Provider.Kind != "deepseek" || got[1].Provider.Kind != "deepseek" ||
		got[0].Home != currentHome || got[1].Home != currentHome {
		t.Fatalf("provider gateway pool = %#v", got)
	}
}
