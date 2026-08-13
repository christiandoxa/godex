package runtime

import (
	"context"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
)

type fakeDoctorAccounts struct {
	prepared bool
	accounts []accountentity.Account
}

func (fake *fakeDoctorAccounts) Prepare() error {
	fake.prepared = true
	return nil
}

func (fake *fakeDoctorAccounts) Root() string { return "/godex" }

func (fake *fakeDoctorAccounts) List(context.Context) ([]accountentity.Account, error) {
	return fake.accounts, nil
}

type fakeVersionedCodex struct{}

func (fakeVersionedCodex) Version(context.Context) (string, error) { return "codex synthetic", nil }

func TestDoctorReportsAccountHealth(t *testing.T) {
	accounts := &fakeDoctorAccounts{accounts: []accountentity.Account{
		{Enabled: true},
		{Enabled: false},
	}}
	report, err := NewDoctor(accounts, fakeVersionedCodex{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !accounts.prepared || report.GodexHome != "/godex" || report.CodexVersion != "codex synthetic" || report.AccountCount != 2 || report.EnabledCount != 1 {
		t.Fatalf("doctor report = %#v, prepared = %t", report, accounts.prepared)
	}
}
