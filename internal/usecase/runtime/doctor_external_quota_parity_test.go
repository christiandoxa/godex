package runtime

import (
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestDoctorExternalQuotaMapsProviderSnapshotWithoutFlattening(t *testing.T) {
	doctor := NewDoctor(&fakeDoctorAccounts{}, fakeVersionedCodex{})
	doctor.SetQuota(fakeDoctorQuota{reports: []quotamodel.Report{{
		ProfileName: "work", Provider: "openai", Auth: "api-key", State: "configured",
		External: &quotamodel.ExternalInfo{
			Status: "Configured", Main: "quota handled by provider/Codex", Reset: "monthly",
		},
	}}})
	report, err := doctor.Diagnose(t.Context(), runtimemodel.DoctorOptions{Quota: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Quota) != 1 || report.Quota[0].External == nil {
		t.Fatalf("doctor quota = %#v", report.Quota)
	}
	external := report.Quota[0].External
	if external.Status != "Configured" || external.Main != "quota handled by provider/Codex" || external.Reset != "monthly" {
		t.Fatalf("external quota = %#v", external)
	}
}
