package openai

import (
	"context"
	"testing"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestNewProxyFromModelAdaptsAccounts(t *testing.T) {
	proxy, err := NewProxyFromModel(proxyconfig.Config{
		UpstreamURL:      "http://upstream.test/backend-api",
		PreferredAccount: "synthetic",
		Accounts: func(context.Context) ([]proxyconfig.Account, error) {
			return []proxyconfig.Account{{ID: "synthetic", Home: "/synthetic/home", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := proxy.source(context.Background())
	if err != nil || len(accounts) != 1 || accounts[0].ID != "synthetic" || accounts[0].Home != "/synthetic/home" || !accounts[0].Enabled {
		t.Fatalf("adapted accounts = %#v, err = %v", accounts, err)
	}
}

func TestNewProxyFromModelRequiresAccountSource(t *testing.T) {
	if _, err := NewProxyFromModel(proxyconfig.Config{}); err == nil {
		t.Fatal("missing account source unexpectedly accepted")
	}
}
