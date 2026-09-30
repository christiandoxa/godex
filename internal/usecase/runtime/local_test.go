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

type disablingLaunchAccounts struct {
	activeLaunchAccounts
	disableID string
	released  bool
}

func (accounts *disablingLaunchAccounts) AcquireProfiles(context.Context, []string) (func() error, error) {
	for i := range accounts.accounts {
		if accounts.accounts[i].ID == accounts.disableID {
			accounts.accounts[i].Enabled = false
		}
	}
	return func() error { accounts.released = true; return nil }, nil
}

func TestLaunchRechecksEligibilityAfterLease(t *testing.T) {
	for _, mode := range []string{"fresh", "picker", "session"} {
		t.Run(mode, func(t *testing.T) {
			accounts := &disablingLaunchAccounts{activeLaunchAccounts: activeLaunchAccounts{&fakeLaunchAccounts{
				accounts: []accountentity.Account{{ID: "one", Enabled: true}}, homes: map[string]string{"one": "/one"},
			}}, disableID: "one"}
			process := &fakeProcess{}
			runner := NewRunner(accounts, process, nil)
			var err error
			switch mode {
			case "fresh":
				err = runner.Run(t.Context(), "one", nil)
			case "picker":
				err = runner.RunCurrent(t.Context(), "one", nil)
			case "session":
				err = runner.RunSession(t.Context(), "one", "one", nil)
			}
			if err == nil || len(process.homes) != 0 || !accounts.released {
				t.Fatalf("stale eligibility launched: error=%v, homes=%v, released=%v", err, process.homes, accounts.released)
			}
		})
	}
}

func TestLaunchRefreshesAlternativeEligibilityAfterLease(t *testing.T) {
	accounts := &disablingLaunchAccounts{activeLaunchAccounts: activeLaunchAccounts{&fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "one", Enabled: true}, {ID: "two", Enabled: true}},
		homes:    map[string]string{"one": "/one", "two": "/two"},
	}}, disableID: "two"}
	var config proxymodel.Config
	runner := NewRunner(accounts, &fakeProxyProcess{}, func(got proxymodel.Config) (Proxy, error) { config = got; return &fakeProxy{}, nil })
	if err := runner.Run(t.Context(), "", nil); err != nil {
		t.Fatal(err)
	}
	profiles, err := config.Accounts(t.Context())
	if err != nil || len(profiles) != 2 || !profiles[0].Enabled || profiles[1].Enabled {
		t.Fatalf("stale alternative eligibility: %v, %v", profiles, err)
	}
}
