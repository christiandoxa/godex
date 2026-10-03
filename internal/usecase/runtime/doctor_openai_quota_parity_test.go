package runtime

import (
	"fmt"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestDoctorOpenAIQuotaMatchesProdexWindowFormatting(t *testing.T) {
	used20, used40 := int64(20), int64(40)
	fiveHour, weekly := int64(18_000), int64(604_800)
	usage := quotamodel.Usage{
		Primary:   &quotamodel.Window{UsedPercent: &used20, LimitWindowSeconds: &fiveHour},
		Secondary: &quotamodel.Window{UsedPercent: &used40, LimitWindowSeconds: &weekly},
	}
	quota := doctorOpenAIQuota(usage)
	if quota == nil || quota.Status != "ready" || quota.HumanStatus != "Ready" ||
		quota.Main != "5h: 80% left (20% used), resets - | weekly: 60% left (40% used), resets -" {
		t.Fatalf("ready OpenAI doctor quota = %#v", quota)
	}

	used100 := int64(100)
	usage.Primary.UsedPercent = &used100
	quota = doctorOpenAIQuota(usage)
	if quota == nil || quota.Status != "blocked" ||
		quota.HumanStatus != "Blocked (5h exhausted until -)" {
		t.Fatalf("blocked OpenAI doctor quota = %#v", quota)
	}
}

func TestDoctorOpenAIQuotaIsUsedByDiagnostics(t *testing.T) {
	used20, used40 := int64(20), int64(40)
	fiveHour, weekly := int64(18_000), int64(604_800)
	doctor := NewDoctor(&fakeDoctorAccounts{}, fakeVersionedCodex{})
	doctor.SetQuota(fakeDoctorQuota{reports: []quotamodel.Report{{
		ProfileName: "work", Provider: "openai", Auth: "chatgpt", State: "ready", Active: true, Enabled: true,
		Usage: quotamodel.Usage{
			Primary:   &quotamodel.Window{UsedPercent: &used20, LimitWindowSeconds: &fiveHour},
			Secondary: &quotamodel.Window{UsedPercent: &used40, LimitWindowSeconds: &weekly},
		},
	}}})
	report, err := doctor.Diagnose(t.Context(), runtimemodel.DoctorOptions{Quota: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Quota) != 1 || report.Quota[0].OpenAI == nil || report.Quota[0].OpenAI.Status != "ready" {
		t.Fatalf("diagnostic OpenAI quota = %#v", report.Quota)
	}
}

func TestDoctorQuotaWindowLabelsMatchProdexBoundaries(t *testing.T) {
	cases := []struct {
		seconds *int64
		want    string
	}{
		{nil, "usage"},
		{doctorInt64(17_700), "5h"},
		{doctorInt64(18_300), "5h"},
		{doctorInt64(601_200), "weekly"},
		{doctorInt64(608_400), "weekly"},
		{doctorInt64(2_505_600), "monthly"},
		{doctorInt64(2_678_400), "monthly"},
		{doctorInt64(42), "42s"},
	}
	for _, test := range cases {
		if got := doctorQuotaWindowLabel(test.seconds); got != test.want {
			t.Errorf("window label for %v = %q, want %q", test.seconds, got, test.want)
		}
	}
}

func TestDoctorOpenAIQuotaDerivesStatusFromUsageNotReportState(t *testing.T) {
	used20, used40, used100 := int64(20), int64(40), int64(100)
	fiveHour, weekly := int64(18_000), int64(604_800)
	usage := quotamodel.Usage{
		Primary:   &quotamodel.Window{UsedPercent: &used20, LimitWindowSeconds: &fiveHour},
		Secondary: &quotamodel.Window{UsedPercent: &used40, LimitWindowSeconds: &weekly},
	}
	ready := doctorOpenAIQuota(usage)
	if ready == nil || ready.Status != "ready" || ready.HumanStatus != "Ready" {
		t.Fatalf("ready quota from usage = %#v", ready)
	}
	usage.Primary.UsedPercent = &used100
	blocked := doctorOpenAIQuota(usage)
	if blocked == nil || blocked.Status != "blocked" || blocked.HumanStatus != "Blocked (5h exhausted until -)" {
		t.Fatalf("blocked quota from usage = %#v", blocked)
	}
}

func TestDoctorOpenAIQuotaMatchesProdexAdmissionAndMainWindowPolicy(t *testing.T) {
	used20, used40, used100 := int64(20), int64(40), int64(100)
	fiveHour, weekly, monthly := int64(18_000), int64(604_800), int64(2_592_000)
	denied := false
	quota := doctorOpenAIQuota(quotamodel.Usage{
		Allowed:   &denied,
		Primary:   &quotamodel.Window{UsedPercent: &used20, LimitWindowSeconds: &fiveHour},
		Secondary: &quotamodel.Window{UsedPercent: &used40, LimitWindowSeconds: &weekly},
	})
	if quota == nil || quota.Status != "blocked" || quota.HumanStatus != "Blocked (quota unavailable)" {
		t.Fatalf("admission-denied quota = %#v", quota)
	}

	quota = doctorOpenAIQuota(quotamodel.Usage{
		Primary: &quotamodel.Window{UsedPercent: &used100, LimitWindowSeconds: &monthly},
	})
	if quota == nil || quota.Status != "blocked" || quota.HumanStatus != "Blocked (quota unavailable)" {
		t.Fatalf("monthly-only blocked quota = %#v", quota)
	}
}

func doctorInt64(value int64) *int64 { return &value }

func TestDoctorOpenAIQuotaDistinguishesMissingAndEmptyRateLimit(t *testing.T) {
	missing := doctorOpenAIQuota(quotamodel.Usage{})
	if missing.HumanStatus != "Blocked (5h quota unavailable, weekly quota unavailable)" {
		t.Fatalf("missing rate-limit quota = %#v", missing)
	}
	empty := doctorOpenAIQuota(quotamodel.Usage{RateLimitPresent: true})
	if empty.HumanStatus != "Blocked (quota unavailable)" {
		t.Fatalf("empty rate-limit quota = %#v", empty)
	}
}

func TestDoctorQuotaErrorUsesProdexRedactedFirstLineSummary(t *testing.T) {
	secret := "sk-" + "aaaaaaaaaaaaaaaaaaaaaaaa"
	doctor := NewDoctor(&fakeDoctorAccounts{}, fakeVersionedCodex{})
	doctor.SetQuota(fakeDoctorQuota{reports: []quotamodel.Report{{
		ProfileName: "work", Provider: "openai", Auth: "chatgpt", Active: true, Enabled: true,
		Err: fmt.Errorf("failed: Authorization: Bearer %s\nsecond diagnostic", secret),
	}}})
	report, err := doctor.Diagnose(t.Context(), runtimemodel.DoctorOptions{Quota: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Quota) != 1 || report.Quota[0].Error != "Error (failed: Authorization: Bearer <redacted>)" {
		t.Fatalf("quota error summary = %#v", report.Quota)
	}
}
