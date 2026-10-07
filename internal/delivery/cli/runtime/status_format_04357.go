package runtime

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func statusProfileField(overview runtimemodel.Overview) string {
	return fmt.Sprintf(
		"runtime=%s, configured=%s, pool=%d, quota-compatible=%d, unavailable=%d",
		valueOrDash(overview.RuntimeProfile),
		valueOrDash(overview.ActiveProfile),
		overview.ProfileCount,
		overview.Quota.CompatibleProfiles,
		overview.Quota.UnavailableProfiles,
	)
}

func statusPoolRemaining(window quotamodel.StatusWindowSummary) string {
	if window.Profiles == 0 {
		return "Unavailable"
	}
	value := fmt.Sprintf("%d%% across %d profile(s)", window.TotalRemaining, window.Profiles)
	if window.EarliestResetAt > 0 && window.EarliestResetAt != math.MaxInt64 {
		value += "; earliest reset " + time.Unix(window.EarliestResetAt, 0).In(time.Local).Format("2006-01-02 15:04:05")
	}
	return value
}

func statusRunway(
	window quotamodel.StatusWindowSummary,
	estimate *runtimemodel.RunwayEstimate,
	now int64,
) string {
	if window.Profiles == 0 {
		return "Unavailable"
	}
	if window.TotalRemaining <= 0 {
		return "Exhausted"
	}
	if estimate == nil {
		return "Unavailable (no recent quota decay observed in active runtime logs)"
	}
	observed := statusRelativeDuration(estimate.ObservedSpanSeconds)
	burn := fmt.Sprintf("%.1f", estimate.BurnPerHour)
	exhaustText := time.Unix(estimate.ExhaustAt, 0).In(time.Local).Format("2006-01-02 15:04:05")
	runway := statusRelativeDuration(max(int64(0), estimate.ExhaustAt-now))
	if window.EarliestResetAt > 0 && window.EarliestResetAt <= estimate.ExhaustAt {
		resetText := time.Unix(window.EarliestResetAt, 0).In(time.Local).Format("2006-01-02 15:04:05")
		return fmt.Sprintf(
			"Earliest reset %s arrives before the no-reset runway (~%s at %s aggregated-%%/h, %d profile(s), observed over %s)",
			resetText, runway, burn, estimate.ObservedProfiles, observed,
		)
	}
	return fmt.Sprintf(
		"%s (~%s) at %s aggregated-%%/h from %d profile(s), observed over %s, no-reset estimate",
		exhaustText, runway, burn, estimate.ObservedProfiles, observed,
	)
}

func statusTokenUsage(summary runtimemodel.TokenUsageSummary) string {
	if summary.EventCount == 0 {
		return fmt.Sprintf("No token_usage events found in %d recent runtime log(s)", summary.LogCount)
	}
	total := summary.Total
	value := fmt.Sprintf(
		"%d event(s), logs=%d: input=%d, cached_input=%d, output=%d, reasoning=%d",
		summary.EventCount, summary.LogCount,
		total.InputTokens, total.CachedInputTokens, total.OutputTokens, total.ReasoningTokens,
	)
	keys := make([]string, 0, len(summary.ByProfile))
	for key := range summary.ByProfile {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 4 {
		keys = keys[:4]
	}
	if len(keys) > 0 {
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			counts := summary.ByProfile[key]
			parts = append(parts, fmt.Sprintf(
				"%s:%d in/%d cached/%d out/%d reasoning",
				key, counts.InputTokens, counts.CachedInputTokens, counts.OutputTokens, counts.ReasoningTokens,
			))
		}
		value += "; by profile: " + strings.Join(parts, "; ")
	}
	return value
}

func statusTokenEfficiency(total runtimemodel.TokenUsageCounts) string {
	cache := 0.0
	if total.InputTokens != 0 {
		cache = float64(total.CachedInputTokens) / float64(total.InputTokens) * 100
	}
	denominator := total.InputTokens
	if math.MaxUint64-total.InputTokens >= total.OutputTokens {
		denominator += total.OutputTokens
	} else {
		denominator = math.MaxUint64
	}
	output := 0.0
	if denominator != 0 {
		output = float64(total.OutputTokens) / float64(denominator) * 100
	}
	return fmt.Sprintf("cache hit %.1f%% · output share %.1f%%", cache, output)
}

func statusSparkline(values []uint64) string {
	if len(values) == 0 {
		return "-"
	}
	maximum := uint64(0)
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	if maximum == 0 {
		return "-"
	}
	levels := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	var builder strings.Builder
	for _, value := range values {
		scaled := uint64(0)
		if value > 0 {
			scaled = uint64((uint64(value) * 7) / maximum)
		}
		if scaled > 7 {
			scaled = 7
		}
		builder.WriteString(levels[scaled])
	}
	return builder.String()
}

func statusTokenHistory(overview runtimemodel.Overview) string {
	first := overview.TokenFirstAt
	if first == "" {
		first = "-"
	}
	last := overview.TokenLastAt
	if last == "" {
		last = "-"
	}
	return fmt.Sprintf("%s %s → %s", statusSparkline(overview.TokenHistory), first, last)
}

func statusLoadSummary(summary runtimemodel.RuntimeLoadSummary, runtimeProcessCount int) string {
	if runtimeProcessCount == 0 {
		return "No active godex runtime detected"
	}
	if summary.LogCount == 0 {
		return "Runtime process detected, but no matching runtime log was found"
	}
	if summary.RecentSelectionEvents == 0 {
		return fmt.Sprintf(
			"%d active runtime log(s); no selection activity observed in the sampled window; inflight units %d",
			summary.LogCount, summary.ActiveInflightUnits,
		)
	}
	if summary.RecentSelectionEvents == 1 {
		return fmt.Sprintf(
			"1 selection event observed in the sampled window; inflight units %d; %d active runtime log(s)",
			summary.ActiveInflightUnits, summary.LogCount,
		)
	}
	span := int64(30 * time.Minute / time.Second)
	if summary.RecentFirstUnixMilli != 0 && summary.RecentLastUnixMilli != 0 {
		span = max(int64(0), (summary.RecentLastUnixMilli-summary.RecentFirstUnixMilli)/1000)
	}
	return fmt.Sprintf(
		"%d selection event(s) over %s; inflight units %d; %d active runtime log(s)",
		summary.RecentSelectionEvents, statusRelativeDuration(span),
		summary.ActiveInflightUnits, summary.LogCount,
	)
}

func statusRelativeDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	switch {
	case seconds == 0:
		return "now"
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return "<1m"
	}
}

func statusRuntimeProcessCount(resources statusResourceSnapshot) int {
	if !resources.available {
		return 0
	}
	return resources.runtimeProcessCount
}
