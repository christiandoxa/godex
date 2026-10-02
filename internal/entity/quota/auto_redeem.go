package quota

import (
	"errors"
	"math"
	"strings"
)

const (
	AutoRedeemMaxCandidates            = 256
	AutoRedeemMaxPlanBytes             = 4096
	AutoRedeemNaturalResetGraceSeconds = int64(300)
	AutoRedeemWeeklyStatusExhausted    = int64(3)
)

type AutoRedeemCandidate struct {
	PlanType       string
	AvailableCount int64
	WeeklyStatus   int64
	WeeklyResetAt  int64
	InflightCount  int64
	HealthSortKey  int64
	OrderIndex     int64
}

func SelectAutoRedeemCandidate(candidates []AutoRedeemCandidate, now int64) (int, error) {
	if len(candidates) > AutoRedeemMaxCandidates {
		return -1, errors.New("auto-redeem candidate pool exceeds the hard limit")
	}
	selected := -1
	for index, candidate := range candidates {
		if err := validateAutoRedeemCandidate(candidate); err != nil {
			return -1, err
		}
		if !AutoRedeemCandidateEligible(candidate, now) {
			continue
		}
		if selected < 0 || autoRedeemCandidateLess(candidate, candidates[selected], index, selected) {
			selected = index
		}
	}
	return selected, nil
}

func AutoRedeemCandidateEligible(candidate AutoRedeemCandidate, now int64) bool {
	if candidate.AvailableCount <= 0 || candidate.WeeklyStatus != AutoRedeemWeeklyStatusExhausted {
		return false
	}
	if candidate.WeeklyResetAt == math.MaxInt64 {
		return false
	}
	return saturatingSub(candidate.WeeklyResetAt, now) > AutoRedeemNaturalResetGraceSeconds
}

func AutoRedeemModelAllowed(model string) bool {
	switch normalizeModelIdentity(model) {
	case "spark", "gpt53codexspark", "gpt53spark":
		return false
	default:
		return true
	}
}

func validateAutoRedeemCandidate(candidate AutoRedeemCandidate) error {
	if len(candidate.PlanType) > AutoRedeemMaxPlanBytes {
		return errors.New("auto-redeem plan type exceeds the safe size limit")
	}
	if candidate.WeeklyStatus < 0 || candidate.WeeklyStatus > 4 || candidate.InflightCount < 0 || candidate.HealthSortKey < 0 || candidate.OrderIndex < 0 {
		return errors.New("auto-redeem candidate contains invalid routing metadata")
	}
	return nil
}

func autoRedeemCandidateLess(left, right AutoRedeemCandidate, leftIndex, rightIndex int) bool {
	if leftPriority, rightPriority := autoRedeemPlanPriority(left.PlanType), autoRedeemPlanPriority(right.PlanType); leftPriority != rightPriority {
		return leftPriority < rightPriority
	}
	if left.WeeklyResetAt != right.WeeklyResetAt {
		return left.WeeklyResetAt > right.WeeklyResetAt
	}
	if left.InflightCount != right.InflightCount {
		return left.InflightCount < right.InflightCount
	}
	if left.HealthSortKey != right.HealthSortKey {
		return left.HealthSortKey < right.HealthSortKey
	}
	if left.OrderIndex != right.OrderIndex {
		return left.OrderIndex < right.OrderIndex
	}
	return leftIndex < rightIndex
}

func autoRedeemPlanPriority(value string) int64 {
	switch normalizePlanType(value) {
	case "plus":
		return 0
	case "free", "basic":
		return 1
	case "prolite", "pro", "pro5x", "5x", "pro20x", "pro20", "20x", "ultra", "max", "team", "business", "enterprise":
		return 3
	default:
		return 2
	}
}

func normalizePlanType(value string) string {
	value = strings.TrimSpace(value)
	var output strings.Builder
	output.Grow(len(value))
	for _, current := range value {
		switch current {
		case ' ', '-', '_':
			continue
		}
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		output.WriteRune(current)
	}
	return output.String()
}

func normalizeModelIdentity(value string) string {
	var output strings.Builder
	for _, current := range value {
		if current >= 'A' && current <= 'Z' {
			current += 'a' - 'A'
		}
		if current >= 'a' && current <= 'z' || current >= '0' && current <= '9' {
			output.WriteRune(current)
		}
	}
	return output.String()
}

func saturatingSub(left, right int64) int64 {
	if right > 0 && left < math.MinInt64+right {
		return math.MinInt64
	}
	if right < 0 && left > math.MaxInt64+right {
		return math.MaxInt64
	}
	return left - right
}
