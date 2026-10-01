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

type countingUsage struct{ calls int }

func (usage *countingUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	usage.calls++
	return quotamodel.Usage{}, nil
}

func TestStatusDoesNotProbeDisabledAccounts(t *testing.T) {
	disabled := accountentity.Account{ID: "off", Name: "off", Enabled: false}
	usage := &countingUsage{}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{disabled}, current: disabled}, usage)
	reports, err := status.Run(context.Background(), Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if usage.calls != 0 || len(reports) != 1 || reports[0].State != "disabled" {
		t.Fatalf("disabled quota reports = %+v, calls = %d", reports, usage.calls)
	}
}

func TestStatusReadyUsesQuotaPolicy(t *testing.T) {
	used := int64(100)
	reset := time.Unix(1000, 0).Unix()
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/one": {Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}},
	}})
	status.now = func() time.Time { return time.Unix(10, 0) }
	ready, err := status.Ready(context.Background(), account)
	if err != nil || ready {
		t.Fatalf("ready = %t, err = %v", ready, err)
	}
}

func TestAvailabilityUsesObservedResetAndFiniteUnknownRetry(t *testing.T) {
	now := time.Unix(100, 0)
	reset := now.Add(15 * time.Second).Unix()
	used := int64(100)
	account := accountentity.Account{ID: "one", Enabled: true}
	for _, test := range []struct {
		usage    quotamodel.Usage
		deadline time.Time
	}{
		{quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}}, time.Unix(reset, 0)},
		{quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, now.Add(time.Minute)},
	} {
		status := NewStatus(fakeAccounts{}, fakeUsage{byHome: map[string]quotamodel.Usage{"/managed/one": test.usage}})
		status.now = func() time.Time { return now }
		availability, err := status.Availability(context.Background(), account)
		if err != nil || availability.Ready || !availability.RetryAt.Equal(test.deadline) {
			t.Fatalf("availability = %v, %v", availability, err)
		}
	}
}

type fakeRawUsage struct {
	home string
	body []byte
}

func (fake *fakeRawUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	return quotamodel.Usage{}, nil
}

func (fake *fakeRawUsage) FetchRaw(_ context.Context, home string) ([]byte, error) {
	fake.home = home
	return append([]byte(nil), fake.body...), nil
}

func TestStatusRawUsesCurrentAndSelectedAccountHomes(t *testing.T) {
	first := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	second := accountentity.Account{ID: "two", Name: "two", Enabled: false}
	usage := &fakeRawUsage{body: []byte(`{"plan_type":"plus"}`)}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{first, second}, current: first}, usage)

	body, err := status.Raw(context.Background(), "")
	if err != nil || string(body) != `{"plan_type":"plus"}` || usage.home != "/managed/one" {
		t.Fatalf("current raw = %q, home = %q, err = %v", body, usage.home, err)
	}
	body, err = status.Raw(context.Background(), "two")
	if err != nil || string(body) != `{"plan_type":"plus"}` || usage.home != "/managed/two" {
		t.Fatalf("selected raw = %q, home = %q, err = %v", body, usage.home, err)
	}
}
