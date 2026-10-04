package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

type quotaSelectionAccounts struct{ *fakeLaunchAccounts }

func (accounts quotaSelectionAccounts) Current(context.Context) (accountentity.Account, error) {
	if len(accounts.accounts) == 0 {
		return accountentity.Account{}, errors.New("no account")
	}
	return accounts.accounts[0], nil
}

func (accounts quotaSelectionAccounts) Resolve(_ context.Context, selector string) (accountentity.Account, error) {
	for _, account := range accounts.accounts {
		if account.ID == selector || account.Name == selector {
			return account, nil
		}
	}
	return accountentity.Account{}, errors.New("account not found")
}

type quotaSelectionUsage struct {
	byHome map[string]quotamodel.Usage
	calls  []string
}

func (usage *quotaSelectionUsage) Fetch(_ context.Context, home string) (quotamodel.Usage, error) {
	usage.calls = append(usage.calls, home)
	return usage.byHome[home], nil
}

func TestRunSelectsProfileWithOnePercentQuotaThroughAvailability(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	usedZero, usedOnePercent := int64(100), int64(99)
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "exhausted", Name: "exhausted", Enabled: true},
			{ID: "last-percent", Name: "last-percent", Enabled: true},
		},
		homes: map[string]string{
			"exhausted":    "/profiles/exhausted",
			"last-percent": "/profiles/last-percent",
		},
	}
	usage := &quotaSelectionUsage{byHome: map[string]quotamodel.Usage{
		"/profiles/exhausted":    {Primary: &quotamodel.Window{UsedPercent: &usedZero, ResetAt: &reset}},
		"/profiles/last-percent": {Primary: &quotamodel.Window{UsedPercent: &usedOnePercent, ResetAt: &reset}},
	}}
	status := quotausecase.NewStatus(quotaSelectionAccounts{accounts}, usage)
	process := &fakeProcess{}
	runner := NewRunner(accounts, process, nil)
	runner.SetQuotaPreflight(status)

	if err := runner.Run(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if len(process.homes) != 1 || process.homes[0] != "/profiles/last-percent" {
		t.Fatalf("selected homes = %#v", process.homes)
	}
	if len(usage.calls) != 2 || usage.calls[0] != "/profiles/exhausted" || usage.calls[1] != "/profiles/last-percent" {
		t.Fatalf("quota usage calls = %#v", usage.calls)
	}
}

func TestRunStopsWhenActualQuotaAvailabilityIsAllZero(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	used := int64(100)
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{
			{ID: "one", Name: "one", Enabled: true},
			{ID: "two", Name: "two", Enabled: true},
		},
		homes: map[string]string{"one": "/profiles/one", "two": "/profiles/two"},
	}
	usage := &quotaSelectionUsage{byHome: map[string]quotamodel.Usage{
		"/profiles/one": {Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}},
		"/profiles/two": {Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}},
	}}
	process := &fakeProcess{}
	runner := NewRunner(accounts, process, nil)
	runner.SetQuotaPreflight(quotausecase.NewStatus(quotaSelectionAccounts{accounts}, usage))

	if err := runner.Run(context.Background(), "", nil); err == nil {
		t.Fatal("all-zero quota pool launched Codex")
	}
	if len(process.homes) != 0 || len(usage.calls) != 2 {
		t.Fatalf("all-zero launches/quota probes = %#v/%#v", process.homes, usage.calls)
	}
}
