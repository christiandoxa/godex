package runtime

import (
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestProdex043712DoctorSelectionSummaryMatchesMarkerBucketsAndReasons(t *testing.T) {
	got := doctorSelectionSummary([]runtimemodel.Event{
		{Kind: "selection_pick", Fields: map[string]string{"profile": "alpha", "route": "responses"}},
		{Kind: "selection_keep_current", Fields: map[string]string{"profile": "alpha"}},
		{Kind: "selection_skip_current", Fields: map[string]string{
			"profile": "beta", "reason": "", "outcome": "filtered", "route": "responses",
		}},
		{Kind: "local_selection_blocked", Fields: map[string]string{
			"event": "auth", "status": "denied",
		}},
		{Kind: "compact_candidate_exhausted"},
		{Kind: "selection_plan", Fields: map[string]string{"profile": "ignored"}},
	})

	if got.Picked != 1 || got.Kept != 1 || got.Skipped != 1 || got.Blocked != 2 {
		t.Fatalf("selection buckets = %#v", got)
	}
	if got.SelectedProfiles["alpha"] != 2 || got.SelectedRoutes["responses"] != 1 {
		t.Fatalf("selected counts = %#v", got)
	}
	if got.RejectedProfiles["beta"] != 1 || got.RejectedRoutes["responses"] != 1 {
		t.Fatalf("rejected counts = %#v", got)
	}
	for _, reason := range []string{"", "auth", "compact_candidate_exhausted"} {
		if got.RejectionReasons[reason] != 1 {
			t.Fatalf("reason %q counts = %#v", reason, got.RejectionReasons)
		}
	}
	if got.LatestDecision != "compact_candidate_exhausted" {
		t.Fatalf("latest decision = %q", got.LatestDecision)
	}
}

func TestProdex043712DoctorSelectionSummaryNormalizesMessageAndRendersDetail(t *testing.T) {
	got := doctorSelectionSummary([]runtimemodel.Event{
		{Kind: "unknown_marker", Message: "selection_pick route=responses profile=alpha status=ready reason=ok"},
		{Kind: "unknown_marker", Message: "selection_skip_current profile=beta outcome=filtered"},
	})
	if got.Picked != 1 || got.Skipped != 1 || got.SelectedProfiles["alpha"] != 1 || got.RejectedProfiles["beta"] != 1 {
		t.Fatalf("normalized selection summary = %#v", got)
	}
	if got.LatestDecision != "selection_skip_current profile=beta outcome=filtered" {
		t.Fatalf("latest decision = %q", got.LatestDecision)
	}
}

func TestProdex043712DoctorSelectionSummaryTruncatesLatestDetail(t *testing.T) {
	value := "東京" + "x"
	for len([]rune(value)) < 60 {
		value += "x"
	}
	got := doctorSelectionSummary([]runtimemodel.Event{{
		Kind: "selection_pick", Fields: map[string]string{"profile": value},
	}})
	if got.LatestDecision != "selection_pick profile="+string([]rune(value)[:45])+"..." {
		t.Fatalf("latest decision = %q", got.LatestDecision)
	}
}
