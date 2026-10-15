package quota

import (
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex043714AcceptedUsageAdvancesOnlyThatProfilesObservationRevision(t *testing.T) {
	status := NewStatus(fakeAccounts{}, failingUsage{})
	now := time.Unix(50000, 0)
	first := int64(12)
	if status.QuotaObservationRevision("main") != 0 {
		t.Fatal("unprobed account has a revision")
	}
	gen := status.probeSequence.Add(1)
	if !status.applySuccessfulUsageProbe043714(t.Context(), "main", "/main", quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &first}}, now, gen, true) {
		t.Fatal("new observation not accepted")
	}
	if status.QuotaObservationRevision("main") != gen || status.QuotaObservationRevision("other") != 0 {
		t.Fatal("observation revision leaked to unrelated account")
	}
	if status.applySuccessfulUsageProbe043714(t.Context(), "main", "/main", quotamodel.Usage{}, now, gen, true) {
		t.Fatal("replayed old generation was accepted")
	}
	if status.QuotaObservationRevision("main") != gen {
		t.Fatal("rejected observation bumped revision")
	}
}
