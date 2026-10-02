package quota

import (
	"math"
	"strings"
	"testing"
)

func TestSelectAutoRedeemCandidateMatchesProdexOrdering(t *testing.T) {
	now := int64(1_000)
	candidates := []AutoRedeemCandidate{
		{PlanType: "pro", AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: now + 10_000, InflightCount: 0, HealthSortKey: 0, OrderIndex: 0},
		{PlanType: "plus", AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: now + 2_000, InflightCount: 9, HealthSortKey: 9, OrderIndex: 9},
		{PlanType: "plus", AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: now + 5_000, InflightCount: 5, HealthSortKey: 5, OrderIndex: 5},
		{PlanType: " free ", AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: now + 20_000, InflightCount: 0, HealthSortKey: 0, OrderIndex: 0},
	}
	selected, err := SelectAutoRedeemCandidate(candidates, now)
	if err != nil || selected != 2 {
		t.Fatalf("selected = %d, err = %v", selected, err)
	}
}

func TestSelectAutoRedeemCandidateTieBreakers(t *testing.T) {
	now := int64(1_000)
	base := AutoRedeemCandidate{PlanType: "unknown", AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: now + 5_000}
	fixtures := []struct {
		name  string
		left  AutoRedeemCandidate
		right AutoRedeemCandidate
	}{
		{"inflight", withInflight(base, 1), withInflight(base, 2)},
		{"health", withHealth(base, 1), withHealth(base, 2)},
		{"order", withOrder(base, 1), withOrder(base, 2)},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			selected, err := SelectAutoRedeemCandidate([]AutoRedeemCandidate{fixture.right, fixture.left}, now)
			if err != nil || selected != 1 {
				t.Fatalf("selected = %d, err = %v", selected, err)
			}
		})
	}
	selected, err := SelectAutoRedeemCandidate([]AutoRedeemCandidate{base, base}, now)
	if err != nil || selected != 0 {
		t.Fatalf("stable tie = %d, err = %v", selected, err)
	}
}

func TestAutoRedeemCandidateEligibilityMatchesProdexGrace(t *testing.T) {
	now := int64(1_000)
	base := AutoRedeemCandidate{AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: now + 301}
	if !AutoRedeemCandidateEligible(base, now) {
		t.Fatal("eligible candidate rejected")
	}
	for _, candidate := range []AutoRedeemCandidate{
		{AvailableCount: 0, WeeklyStatus: 3, WeeklyResetAt: now + 10_000},
		{AvailableCount: 1, WeeklyStatus: 2, WeeklyResetAt: now + 10_000},
		{AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: now + 300},
		{AvailableCount: 1, WeeklyStatus: 3, WeeklyResetAt: math.MaxInt64},
	} {
		if AutoRedeemCandidateEligible(candidate, now) {
			t.Fatalf("ineligible candidate accepted: %#v", candidate)
		}
	}
}

func TestAutoRedeemPlanPriorityNormalizationMatchesProdex(t *testing.T) {
	for value, want := range map[string]int64{
		"PLUS": 0, " free ": 1, "ba-s_ic": 1, "future": 2,
		"pro": 3, "pro-lite": 3, "pro_5x": 3, "20x": 3, "business": 3, "enterprise": 3,
	} {
		if got := autoRedeemPlanPriority(value); got != want {
			t.Fatalf("priority(%q) = %d, want %d", value, got, want)
		}
	}
}

func TestAutoRedeemPlannerRejectsInvalidInputs(t *testing.T) {
	if _, err := SelectAutoRedeemCandidate(make([]AutoRedeemCandidate, AutoRedeemMaxCandidates+1), 0); err == nil {
		t.Fatal("oversized candidate pool accepted")
	}
	for _, candidate := range []AutoRedeemCandidate{
		{PlanType: strings.Repeat("x", AutoRedeemMaxPlanBytes+1)},
		{WeeklyStatus: -1},
		{WeeklyStatus: 5},
		{InflightCount: -1},
		{HealthSortKey: -1},
		{OrderIndex: -1},
	} {
		if _, err := SelectAutoRedeemCandidate([]AutoRedeemCandidate{candidate}, 0); err == nil {
			t.Fatalf("invalid candidate accepted: %#v", candidate)
		}
	}
}

func TestAutoRedeemModelAllowedRejectsRetiredSparkAliases(t *testing.T) {
	for _, model := range []string{"spark", "SPARK", "gpt-5.3-codex-spark", "gpt_5_3_spark"} {
		if AutoRedeemModelAllowed(model) {
			t.Fatalf("retired Spark model %q accepted", model)
		}
	}
	for _, model := range []string{"", "gpt-5.6-luna", "gpt-5.3-codex", "o3"} {
		if !AutoRedeemModelAllowed(model) {
			t.Fatalf("valid model %q rejected", model)
		}
	}
}

func withInflight(candidate AutoRedeemCandidate, value int64) AutoRedeemCandidate {
	candidate.InflightCount = value
	return candidate
}
func withHealth(candidate AutoRedeemCandidate, value int64) AutoRedeemCandidate {
	candidate.HealthSortKey = value
	return candidate
}
func withOrder(candidate AutoRedeemCandidate, value int64) AutoRedeemCandidate {
	candidate.OrderIndex = value
	return candidate
}
