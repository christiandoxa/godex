package ping

import (
	"context"
	"testing"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

// Prodex 0.435.8 derives top-level JSON selection fields from the first
// profile result. With zero matching OpenAI profiles they must be null,
// even when --model/--effort were provided.
func TestProdex04358PingNoProfilesOmitsRequestedSelection(t *testing.T) {
	probe := NewOpenAI(fakePingProfiles{}, fakePingProcess{})
	report, err := probe.Run(context.Background(), pingmodel.Options{
		Model:  "gpt-5.6-luna",
		Effort: "MAX",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" || len(report.Profiles) != 0 {
		t.Fatalf("empty profile report = %+v", report)
	}
	if report.Model != "" || report.RequestedModel != "" || report.Effort != "" ||
		report.RequestedEffort != "" || report.EffectiveModel != "" {
		t.Fatalf("empty profiles leaked requested selection into JSON report: %+v", report)
	}
}
