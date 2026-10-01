package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
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

type fakeVersionedCodex struct{ supportErr error }

func (fakeVersionedCodex) Version(context.Context) (string, error) { return "codex synthetic", nil }

func (fake fakeVersionedCodex) CheckProxySupport(context.Context) error { return fake.supportErr }

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

func TestDoctorRejectsUnsupportedCodex(t *testing.T) {
	want := errors.New("unsupported Codex runtime")
	report, err := NewDoctor(&fakeDoctorAccounts{}, fakeVersionedCodex{supportErr: want}).Run(t.Context())
	if !errors.Is(err, want) || report.CodexVersion != "" {
		t.Fatalf("doctor report = %#v, error = %v", report, err)
	}
}

type fakeDoctorActivity struct {
	overview runtimemodel.Overview
	events   []runtimemodel.Event
}

func (fake fakeDoctorActivity) Overview(context.Context) (runtimemodel.Overview, error) {
	return fake.overview, nil
}

func (fake fakeDoctorActivity) Events(context.Context, int) ([]runtimemodel.Event, error) {
	return append([]runtimemodel.Event(nil), fake.events...), nil
}

type fakeDoctorQuota struct {
	reports []quotamodel.Report
}

func (fake fakeDoctorQuota) DoctorReports(context.Context) ([]quotamodel.Report, error) {
	return append([]quotamodel.Report(nil), fake.reports...), nil
}

func TestDoctorDiagnoseAddsBoundedRuntimeQuotaAndInstallData(t *testing.T) {
	used := int64(20)
	doctor := NewDoctor(&fakeDoctorAccounts{accounts: []accountentity.Account{{Enabled: true}}}, fakeVersionedCodex{})
	doctor.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	doctor.SetActivity(fakeDoctorActivity{
		overview: runtimemodel.Overview{GodexHome: "/godex", ActiveProfile: "work", ProfileCount: 2, RecentEvents: 2},
		events: []runtimemodel.Event{
			{Kind: "request_started", RequestID: "a", AccountID: "sensitive-account-id"},
			{Kind: "request_completed", RequestID: "a", AccountID: "sensitive-account-id", StatusCode: 200},
		},
	})
	doctor.SetQuota(fakeDoctorQuota{reports: []quotamodel.Report{{
		ProfileName: "work", Provider: "openai", Auth: "chatgpt", State: "ready", Active: true, Enabled: true,
		Usage: quotamodel.Usage{PlanType: "plus", Primary: &quotamodel.Window{UsedPercent: &used}},
	}}})
	report, err := doctor.Diagnose(context.Background(), runtimemodel.DoctorOptions{Runtime: true, Quota: true, Install: true, TailBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if report.Runtime == nil || len(report.Runtime.Events) != 2 || len(report.Install) != 3 || len(report.Quota) != 1 {
		t.Fatalf("diagnostics = %+v", report)
	}
	if report.Runtime.Events[0].AccountID != "" || report.Quota[0].FiveHour != "80%" || report.Quota[0].Plan != "plus" {
		t.Fatalf("redaction/quota = %+v / %+v", report.Runtime.Events, report.Quota)
	}
}

func TestDoctorTailEventsHonorsByteBudget(t *testing.T) {
	events := []runtimemodel.Event{
		{Kind: "request_started", RequestID: strings.Repeat("a", 64)},
		{Kind: "request_completed", RequestID: strings.Repeat("b", 64)},
	}
	all := doctorTailEvents(events, 4096)
	if len(all) != 2 {
		t.Fatalf("all events = %d", len(all))
	}
	if got := doctorTailEvents(events, 1); len(got) != 0 {
		t.Fatalf("tiny tail returned %d events", len(got))
	}
}
