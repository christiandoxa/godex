package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type fakeAccounts struct {
	accounts []accountentity.Account
	current  accountentity.Account
}

func (fake fakeAccounts) List(context.Context) ([]accountentity.Account, error) {
	return append([]accountentity.Account(nil), fake.accounts...), nil
}
func (fake fakeAccounts) Current(context.Context) (accountentity.Account, error) {
	if fake.current.ID == "" {
		return accountentity.Account{}, errors.New("no current")
	}
	return fake.current, nil
}
func (fake fakeAccounts) Resolve(_ context.Context, selector string) (accountentity.Account, error) {
	for _, account := range fake.accounts {
		if account.Name == selector {
			return account, nil
		}
	}
	return accountentity.Account{}, errors.New("not found")
}
func (fake fakeAccounts) CodexHome(id string) string { return "/managed/" + id }

type fakeUsage struct{ byHome map[string]quotamodel.Usage }

func (fake fakeUsage) Fetch(_ context.Context, home string) (quotamodel.Usage, error) {
	usage, ok := fake.byHome[home]
	if !ok {
		return quotamodel.Usage{}, errors.New("probe failed")
	}
	return usage, nil
}

func TestStatusReportsAllAccountsAndQuotaState(t *testing.T) {
	used100 := int64(100)
	future := time.Unix(1000, 0).Unix()
	first := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	second := accountentity.Account{ID: "two", Name: "two", Enabled: true}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{first, second}, current: first}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/one": {PlanType: "plus"},
		"/managed/two": {Primary: &quotamodel.Window{UsedPercent: &used100, ResetAt: &future}},
	}})
	status.now = func() time.Time { return time.Unix(10, 0) }
	reports, err := status.Run(context.Background(), Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 || reports[0].State != "ready" || !reports[0].Active || reports[1].State != "exhausted" {
		t.Fatalf("reports = %+v", reports)
	}
}

func TestStatusDefaultsToCurrentAccount(t *testing.T) {
	current := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{current}, current: current}, fakeUsage{byHome: map[string]quotamodel.Usage{"/managed/one": {PlanType: "plus"}}})
	reports, err := status.Run(context.Background(), Options{})
	if err != nil || len(reports) != 1 || reports[0].AccountName != "one" {
		t.Fatalf("reports = %+v, err = %v", reports, err)
	}
}
