package quota

import (
	"errors"
	"reflect"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Prodex 0.437.9 derives the OpenAI sort rank from quota-capacity
// candidates, not the separately rendered human-readable State string.
func TestProdex04379QuotaSortUsesWindowCapacityInsteadOfStatusText(t *testing.T) {
	twenty := int64(20)
	hundred := int64(100)
	denied := false
	reports := []quotamodel.Report{
		{ProfileName: "a-unknown", Provider: "openai", State: "ready"},
		{ProfileName: "b-blocked", Provider: "openai", State: "ready",
			Usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &hundred}}},
		{ProfileName: "m-admission-denied", Provider: "openai", State: "ready",
			Usage: quotamodel.Usage{Allowed: &denied, Primary: &quotamodel.Window{UsedPercent: &twenty}}},
		{ProfileName: "z-ready", Provider: "openai", State: "exhausted",
			Usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &twenty}}},
		{ProfileName: "x-error", Provider: "openai", State: "ready",
			Err: errors.New("synthetic quota read failure")},
	}
	for _, tc := range []struct {
		index int
		rank  int
	}{
		{0, 1}, // No observed capacity is not ready.
		{1, 1}, // Explicit zero remaining capacity is blocked.
		{2, 1}, // Admission denied despite nonexhausted window.
		{3, 0}, // A ready window controls the sort, not stale State text.
		{4, 2}, // Fetch errors sort last.
	} {
		if got := quotaReportStatusRank(reports[tc.index]); got != tc.rank {
			t.Errorf("%q rank = %d, want %d", reports[tc.index].ProfileName, got, tc.rank)
		}
	}
	sorted := quotaSortedReports(reports, quotaProviderAll, quotaSortRemaining)
	var names []string
	for _, item := range sorted {
		names = append(names, item.ProfileName)
	}
	want := []string{"z-ready", "a-unknown", "b-blocked", "m-admission-denied", "x-error"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tagged 0.437.9 remaining order: got=%v want=%v", names, want)
	}
}

func TestProdex04379QuotaSortOnlyUsesRelevantProviderResetEpoch(t *testing.T) {
	early, late := int64(100), int64(900)
	available := true
	reports := []quotamodel.Report{
		{ProfileName: "beta", Provider: "deepseek", State: "ready",
			External: &quotamodel.ExternalInfo{Available: &available, ResetAt: &early}},
		{ProfileName: "alpha", Provider: "deepseek", State: "ready",
			External: &quotamodel.ExternalInfo{Available: &available, ResetAt: &late}},
	}
	// The tagged Prodex report comparator assigns MAX reset epoch to its
	// External provider variant; its tie-breaker is the profile name.
	sorted := quotaSortedReports(reports, quotaProviderAll, quotaSortRemaining)
	if sorted[0].ProfileName != "alpha" || sorted[1].ProfileName != "beta" {
		t.Fatalf("generic external report sorted by provider-specific reset, got %q/%q",
			sorted[0].ProfileName, sorted[1].ProfileName)
	}
	// Copilot still has an explicit reset epoch in the tagged renderer.
	reports[0].Provider, reports[1].Provider = "copilot", "copilot"
	sorted = quotaSortedReports(reports, quotaProviderAll, quotaSortRemaining)
	if sorted[0].ProfileName != "beta" || sorted[1].ProfileName != "alpha" {
		t.Fatalf("copilot reset ordering regressed: %q/%q",
			sorted[0].ProfileName, sorted[1].ProfileName)
	}
}
