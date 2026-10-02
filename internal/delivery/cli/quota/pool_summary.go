package quota

import (
	"fmt"
	"math"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const quotaUnavailableLabel = "Unavailable"

type quotaPoolAggregate struct {
	totalProfiles          int
	availableProfiles      int
	openAIProfiles         int
	readyOpenAIProfiles    int
	fiveHourProfiles       int
	weeklyProfiles         int
	readyFiveHourProfiles  int
	readyWeeklyProfiles    int
	fiveHourRemaining      int64
	weeklyRemaining        int64
	readyFiveHourRemaining int64
	readyWeeklyRemaining   int64
	earliestFiveHourReset  *int64
	earliestWeeklyReset    *int64
	mainProfiles           int
	mainRemaining          int64
	earliestMainReset      *int64
}

type quotaPoolField struct {
	label string
	value string
}

func quotaPoolSummaryFields(reports []quotamodel.Report, updated time.Time) []quotaPoolField {
	aggregate := collectQuotaPoolAggregate(reports)
	fields := []quotaPoolField{
		{label: "Available", value: fmt.Sprintf("%d/%d profile", aggregate.availableProfiles, aggregate.totalProfiles)},
		{label: "Last Updated", value: quotaLocalTime(updated)},
	}
	switch {
	case aggregate.openAIProfiles > 0:
		fields = append(fields,
			quotaPoolField{label: "Usable now", value: formatReadyPoolRemaining(aggregate)},
			quotaPoolField{label: "5h remaining pool", value: formatPoolRemaining(aggregate.fiveHourRemaining, aggregate.fiveHourProfiles, aggregate.earliestFiveHourReset)},
			quotaPoolField{label: "Weekly remaining pool", value: formatPoolRemaining(aggregate.weeklyRemaining, aggregate.weeklyProfiles, aggregate.earliestWeeklyReset)},
		)
	case aggregate.mainProfiles > 0:
		fields = append(fields, quotaPoolField{label: "Remaining pool", value: formatPoolRemaining(aggregate.mainRemaining, aggregate.mainProfiles, aggregate.earliestMainReset)})
	default:
		fields = append(fields,
			quotaPoolField{label: "5h remaining pool", value: quotaUnavailableLabel},
			quotaPoolField{label: "Weekly remaining pool", value: quotaUnavailableLabel},
		)
	}
	return fields
}

func collectQuotaPoolAggregate(reports []quotamodel.Report) quotaPoolAggregate {
	aggregate := quotaPoolAggregate{totalProfiles: len(reports)}
	for _, report := range reports {
		if quotaReportAvailableForPool(report) {
			aggregate.availableProfiles++
		}
		if report.Err != nil {
			continue
		}
		if report.Provider == "openai" || report.Provider == "" && report.External == nil {
			aggregateOpenAIQuota(&aggregate, report)
			continue
		}
		if report.External != nil && report.External.RemainingPercent != nil {
			aggregate.mainProfiles++
			aggregate.mainRemaining = saturatingAdd(aggregate.mainRemaining, *report.External.RemainingPercent)
			aggregate.earliestMainReset = earliestReset(aggregate.earliestMainReset, report.External.ResetAt)
		}
	}
	return aggregate
}

func quotaReportAvailableForPool(report quotamodel.Report) bool {
	if report.Err != nil {
		return false
	}
	if report.External != nil {
		return report.External.Available != nil && *report.External.Available
	}
	if report.Provider == "openai" || report.Provider == "" {
		_, fiveOK := quotaRemainingWindow(report.Usage.Primary)
		_, weeklyOK := quotaRemainingWindow(report.Usage.Secondary)
		return (fiveOK || weeklyOK) && quotaReportStatusRank(report) == 0
	}
	return false
}

func aggregateOpenAIQuota(aggregate *quotaPoolAggregate, report quotamodel.Report) {
	five, fiveOK := quotaRemainingWindow(report.Usage.Primary)
	weekly, weeklyOK := quotaRemainingWindow(report.Usage.Secondary)
	if !fiveOK && !weeklyOK {
		return
	}
	aggregate.openAIProfiles++
	ready := quotaReportStatusRank(report) == 0
	if ready {
		aggregate.readyOpenAIProfiles++
	}
	if fiveOK {
		aggregate.fiveHourProfiles++
		aggregate.fiveHourRemaining = saturatingAdd(aggregate.fiveHourRemaining, five)
		aggregate.earliestFiveHourReset = earliestReset(aggregate.earliestFiveHourReset, report.Usage.Primary.ResetAt)
		if ready {
			aggregate.readyFiveHourProfiles++
			aggregate.readyFiveHourRemaining = saturatingAdd(aggregate.readyFiveHourRemaining, five)
		}
	}
	if weeklyOK {
		aggregate.weeklyProfiles++
		aggregate.weeklyRemaining = saturatingAdd(aggregate.weeklyRemaining, weekly)
		aggregate.earliestWeeklyReset = earliestReset(aggregate.earliestWeeklyReset, report.Usage.Secondary.ResetAt)
		if ready {
			aggregate.readyWeeklyProfiles++
			aggregate.readyWeeklyRemaining = saturatingAdd(aggregate.readyWeeklyRemaining, weekly)
		}
	}
}

func quotaRemainingWindow(window *quotamodel.Window) (int64, bool) {
	if window == nil || window.UsedPercent == nil {
		return 0, false
	}
	remaining := int64(100) - *window.UsedPercent
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	return remaining, true
}

func formatReadyPoolRemaining(aggregate quotaPoolAggregate) string {
	if aggregate.readyOpenAIProfiles == 0 {
		return quotaUnavailableLabel
	}
	parts := make([]string, 0, 2)
	if aggregate.readyFiveHourProfiles > 0 {
		parts = append(parts, fmt.Sprintf("5h %d%%", aggregate.readyFiveHourRemaining))
	}
	if aggregate.readyWeeklyProfiles > 0 {
		parts = append(parts, fmt.Sprintf("weekly %d%%", aggregate.readyWeeklyRemaining))
	}
	return joinPoolParts(parts) + fmt.Sprintf(" across %d ready profile(s)", aggregate.readyOpenAIProfiles)
}

func formatPoolRemaining(total int64, profiles int, reset *int64) string {
	if profiles == 0 {
		return quotaUnavailableLabel
	}
	result := fmt.Sprintf("%d%% across %d profile(s)", total, profiles)
	if reset != nil {
		result += "; earliest reset " + time.Unix(*reset, 0).Local().Format("2006-01-02 15:04:05")
	}
	return result
}

func quotaLocalTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func earliestReset(current, candidate *int64) *int64 {
	if candidate == nil {
		return current
	}
	if current == nil || *candidate < *current {
		copy := *candidate
		return &copy
	}
	return current
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	if right < 0 && left < math.MinInt64-right {
		return math.MinInt64
	}
	return left + right
}

func joinPoolParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return parts[0] + " | " + parts[1]
}
