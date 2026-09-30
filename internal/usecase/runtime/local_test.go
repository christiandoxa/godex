package runtime

import (
	"context"
	"errors"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type activeLaunchAccounts struct{ *fakeLaunchAccounts }

func (accounts activeLaunchAccounts) Current(context.Context) (accountentity.Account, error) {
	return accounts.accounts[0], nil
}
func (accounts activeLaunchAccounts) Resolve(_ context.Context, selector string) (accountentity.Account, error) {
	for _, account := range accounts.accounts {
		if account.Matches(selector) {
			return account, nil
		}
	}
	return accountentity.Account{}, errors.New("missing account")
}

func TestNativePickerKeepsHomeAndRotatedOwnerPoolWithoutQuota(t *testing.T) {
	for _, selector := range []string{"", "home"} {
		accounts := activeLaunchAccounts{&fakeLaunchAccounts{accounts: []accountentity.Account{{ID: "home", Enabled: true}, {ID: "owner", Enabled: true}}, homes: map[string]string{"home": "/rollouts", "owner": "/credentials"}, selectErr: errors.New("fresh selection must not run")}}
		process := &fakeProxyProcess{}
		var config proxymodel.Config
		runner := NewRunner(accounts, process, func(got proxymodel.Config) (Proxy, error) { config = got; return &fakeProxy{}, nil })
		runner.SetQuotaPreflight(&fakeQuotaPreflight{ready: map[string]bool{}})
		if err := runner.RunCurrent(t.Context(), selector, []string{"resume", "--last"}); err != nil {
			t.Fatal(err)
		}
		profiles, err := config.Accounts(t.Context())
		want := 2
		if selector != "" {
			want = 1
		}
		if err != nil || len(profiles) != want || process.home != "/rollouts" {
			t.Fatalf("picker home/pool = %s, %v, %v", process.home, profiles, err)
		}
		if selector != "" && profiles[0].ID != "home" {
			t.Fatal("explicit account scope escaped")
		}
	}
}
