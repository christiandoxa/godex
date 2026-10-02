package quota

import (
	"strings"
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

func TestQuotaPoolSummaryOpenAIMatchesProdexAggregation(t *testing.T) {
	used20, used50, used100 := int64(20), int64(50), int64(100)
	reset100, reset200, reset300 := int64(100), int64(200), int64(300)
	mainRemaining := int64(40)
	available := true
	reports := []quotamodel.Report{
		{ProfileName: "ready", Provider: "openai", State: "ready", Usage: quotamodel.Usage{
			Primary:   &quotamodel.Window{UsedPercent: &used20, ResetAt: &reset200},
			Secondary: &quotamodel.Window{UsedPercent: &used50, ResetAt: &reset300},
		}},
		{ProfileName: "blocked", Provider: "openai", State: "exhausted", Usage: quotamodel.Usage{
			Primary: &quotamodel.Window{UsedPercent: &used100, ResetAt: &reset100},
		}},
		{ProfileName: "copilot", Provider: "copilot", State: "ready", External: &quotamodel.ExternalInfo{
			Available: &available, RemainingPercent: &mainRemaining, ResetAt: &reset100,
		}},
	}
	fields := quotaPoolSummaryFields(reports, time.Unix(400, 0))
	assertQuotaPoolField(t, fields, "Available", "2/3 profile")
	assertQuotaPoolField(t, fields, "Usable now", "5h 80% | weekly 50% across 1 ready profile(s)")
	assertQuotaPoolField(t, fields, "5h remaining pool", "80% across 2 profile(s); earliest reset "+time.Unix(reset100, 0).Local().Format("2006-01-02 15:04:05"))
	assertQuotaPoolField(t, fields, "Weekly remaining pool", "50% across 1 profile(s); earliest reset "+time.Unix(reset300, 0).Local().Format("2006-01-02 15:04:05"))
	if value := quotaPoolFieldValue(fields, "Remaining pool"); value != "" {
		t.Fatalf("main pool should be hidden when OpenAI window data exists: %q", value)
	}
}

func TestQuotaPoolSummaryMainQuotaMatchesProdexAggregation(t *testing.T) {
	remaining40, remaining75 := int64(40), int64(75)
	reset100, reset200 := int64(100), int64(200)
	available := true
	reports := []quotamodel.Report{
		{ProfileName: "one", Provider: "copilot", External: &quotamodel.ExternalInfo{Available: &available, RemainingPercent: &remaining40, ResetAt: &reset200}},
		{ProfileName: "two", Provider: "copilot", External: &quotamodel.ExternalInfo{Available: &available, RemainingPercent: &remaining75, ResetAt: &reset100}},
	}
	fields := quotaPoolSummaryFields(reports, time.Unix(400, 0))
	assertQuotaPoolField(t, fields, "Available", "2/2 profile")
	assertQuotaPoolField(t, fields, "Remaining pool", "115% across 2 profile(s); earliest reset "+time.Unix(reset100, 0).Local().Format("2006-01-02 15:04:05"))
}

func TestQuotaPoolSummaryUnavailableAndLastUpdated(t *testing.T) {
	updated := time.Unix(1_700_000_000, 0)
	fields := quotaPoolSummaryFields([]quotamodel.Report{{ProfileName: "unknown", Provider: "anthropic"}}, updated)
	assertQuotaPoolField(t, fields, "Available", "0/1 profile")
	assertQuotaPoolField(t, fields, "Last Updated", updated.Local().Format("2006-01-02 15:04:05"))
	assertQuotaPoolField(t, fields, "5h remaining pool", "Unavailable")
	assertQuotaPoolField(t, fields, "Weekly remaining pool", "Unavailable")
}

func TestQuotaAllTUIViewIncludesPoolOverview(t *testing.T) {
	status := quotaAllTUITestStatus()
	model := newQuotaTUIModel(t.Context(), status, showOptions{Options: quotausecase.Options{All: true}})
	updated, _ := model.Update(quotaSnapshotMsg{reports: status.reports})
	model = updated.(quotaTUIModel)
	view := model.View()
	for _, expected := range []string{"Quota Overview", "Available:", "Last Updated:", "5h remaining pool:", "Weekly remaining pool:"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q: %q", expected, view)
		}
	}
}

func assertQuotaPoolField(t *testing.T, fields []quotaPoolField, label, want string) {
	t.Helper()
	if got := quotaPoolFieldValue(fields, label); got != want {
		t.Fatalf("field %q = %q, want %q", label, got, want)
	}
}

func quotaPoolFieldValue(fields []quotaPoolField, label string) string {
	for _, field := range fields {
		if field.label == label {
			return field.value
		}
	}
	return ""
}
