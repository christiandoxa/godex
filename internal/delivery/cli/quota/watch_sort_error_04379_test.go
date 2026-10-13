package quota

import (
	"errors"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// The tagged Prodex report's Err result, not a possibly stale display
// status string, decides whether the report sorts below valid quota data.
func TestProdex04379QuotaSortIgnoresStaleErrorLabelWithValidEvidence(t *testing.T) {
	healthy, exhausted := int64(20), int64(100)
	for _, tc := range []struct {
		name     string
		report   quotamodel.Report
		wantRank int
	}{
		{
			name: "stale error label on healthy OpenAI window",
			report: quotamodel.Report{
				Provider: "openai", State: "error",
				Usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &healthy}},
			},
			wantRank: 0,
		},
		{
			name: "weekly exhausted despite healthy five-hour",
			report: quotamodel.Report{
				Provider: "openai", State: "ready",
				Usage: quotamodel.Usage{
					Primary:   &quotamodel.Window{UsedPercent: &healthy},
					Secondary: &quotamodel.Window{UsedPercent: &exhausted},
				},
			},
			wantRank: 1,
		},
		{
			name: "actual quota fetch error always ranks last",
			report: quotamodel.Report{
				Provider: "openai", State: "ready",
				Usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &healthy}},
				Err:   errors.New("synthetic fetch error"),
			},
			wantRank: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotaReportStatusRank(tc.report); got != tc.wantRank {
				t.Fatalf("rank = %d want %d", got, tc.wantRank)
			}
		})
	}
}
