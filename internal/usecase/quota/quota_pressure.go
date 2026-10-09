package quota

import (
	"math"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func quotaPressure(usage quotamodel.Usage, selection quotamodel.Selection, now time.Time) quotamodel.Pressure {
	primary, secondary, _, _ := quotaWindowsForModel(usage, selection, now)
	weekly := quotaWindowPressure(secondary, now)
	fiveHour := quotaWindowPressure(primary, now)
	pressure := quotamodel.Pressure{
		Band: 4, Total: math.MaxInt64, Weekly: math.MaxInt64, FiveHour: math.MaxInt64,
		ReserveFloor: 0, WeeklyRemaining: 0, FiveHourRemaining: 0,
		WeeklyResetAt: math.MaxInt64, FiveHourResetAt: math.MaxInt64,
	}
	if !weekly.known && !fiveHour.known {
		return pressure
	}
	pressure.Known = weekly.known && fiveHour.known
	if weekly.known {
		pressure.Weekly, pressure.WeeklyRemaining, pressure.WeeklyResetAt = weekly.score, weekly.remaining, weekly.resetAt
	}
	if fiveHour.known {
		pressure.FiveHour, pressure.FiveHourRemaining, pressure.FiveHourResetAt = fiveHour.score, fiveHour.remaining, fiveHour.resetAt
	}
	pressure.ReserveFloor = min(pressure.WeeklyRemaining, pressure.FiveHourRemaining)
	pressure.Band = quotaPressureBand(weekly, fiveHour, selection.RouteKind)
	pressure.Weekly = scaleQuotaPressure(pressure.Weekly, quotaPlanScale(usage.PlanType))
	pressure.FiveHour = scaleQuotaPressure(pressure.FiveHour, quotaPlanScale(usage.PlanType))
	weeklyWeight := int64(8)
	if selection.RouteKind == quotamodel.RouteKindResponses || selection.RouteKind == quotamodel.RouteKindWebSocket {
		weeklyWeight = 10
	}
	pressure.Total = saturatingAdd(quotaPressureBandBias(pressure.Band), saturatingAdd(saturatingMultiply(pressure.Weekly, weeklyWeight), pressure.FiveHour))
	return pressure
}

type quotaWindowScore struct {
	known     bool
	remaining int64
	resetAt   int64
	score     int64
}

func quotaWindowPressure(window *quotamodel.Window, now time.Time) quotaWindowScore {
	if window == nil || window.UsedPercent == nil {
		return quotaWindowScore{score: math.MaxInt64, resetAt: math.MaxInt64}
	}
	used := *window.UsedPercent
	remaining := max(int64(0), min(int64(100), 100-used))
	resetAt := int64(math.MaxInt64)
	seconds := int64(math.MaxInt64)
	if window.ResetAt != nil {
		resetAt = *window.ResetAt
		if resetAt <= now.Unix() {
			seconds = 0
		} else {
			seconds = resetAt - now.Unix()
			if seconds < 0 {
				seconds = math.MaxInt64
			}
		}
	}
	denominator := max(int64(1), remaining)
	score := int64(math.MaxInt64)
	if seconds <= math.MaxInt64/1000 {
		score = seconds * 1000 / denominator
	}
	return quotaWindowScore{known: true, remaining: remaining, resetAt: resetAt, score: score}
}

func quotaPressureBand(weekly, fiveHour quotaWindowScore, route quotamodel.RouteKind) uint8 {
	if weekly.known && weekly.remaining == 0 || fiveHour.known && fiveHour.remaining == 0 {
		return 3
	}
	if !weekly.known && !fiveHour.known {
		return 4
	}
	thinWeekly, thinFiveHour, criticalWeekly, criticalFiveHour := int64(10), int64(5), int64(5), int64(3)
	if route == quotamodel.RouteKindResponses || route == quotamodel.RouteKindWebSocket {
		thinWeekly, thinFiveHour, criticalWeekly, criticalFiveHour = 20, 10, 10, 5
	}
	band := uint8(0)
	if weekly.known {
		if weekly.remaining <= criticalWeekly {
			band = 2
		} else if weekly.remaining <= thinWeekly {
			band = 1
		}
	}
	if fiveHour.known {
		if fiveHour.remaining <= criticalFiveHour {
			band = max(band, uint8(2))
		} else if fiveHour.remaining <= thinFiveHour {
			band = max(band, uint8(1))
		}
	}
	return band
}

func quotaPlanScale(plan string) int64 {
	plan = strings.Map(func(value rune) rune {
		if value == ' ' || value == '-' || value == '_' {
			return -1
		}
		return value
	}, strings.ToLower(strings.TrimSpace(plan)))
	switch plan {
	case "pro20x", "pro20", "20x", "ultra", "max":
		return 2_000
	case "pro", "prolite", "pro5x", "5x":
		return 5_000
	case "free", "basic":
		return 12_000
	default:
		return 10_000
	}
}

func quotaPressureBandBias(band uint8) int64 {
	switch band {
	case 1:
		return 250_000
	case 2:
		return 1_000_000
	case 3, 4:
		return math.MaxInt64 / 4
	default:
		return 0
	}
}

func scaleQuotaPressure(pressure, scale int64) int64 {
	if pressure == math.MaxInt64 {
		return pressure
	}
	if pressure == 0 || scale == 0 {
		return 0
	}
	return saturatingMultiply(pressure, scale) / 10_000
}

func saturatingMultiply(value, multiplier int64) int64 {
	if value <= 0 || multiplier <= 0 {
		return 0
	}
	if value > math.MaxInt64/multiplier {
		return math.MaxInt64
	}
	return value * multiplier
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64
	}
	return left + right
}
