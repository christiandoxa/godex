package runtime

import (
	"fmt"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func doctorOpenAIQuota(usage quotamodel.Usage) *runtimemodel.DoctorOpenAIQuota {
	main := doctorOpenAIMainWindows(usage)
	if doctorOpenAIReady(usage) {
		return &runtimemodel.DoctorOpenAIQuota{Status: "ready", HumanStatus: "Ready", Main: main}
	}
	blocked := doctorOpenAIBlockedLimits(usage)
	return &runtimemodel.DoctorOpenAIQuota{
		Status: "blocked", HumanStatus: "Blocked (" + strings.Join(blocked, ", ") + ")", Main: main,
	}
}

func doctorOpenAIReady(usage quotamodel.Usage) bool {
	if usage.Allowed != nil && !*usage.Allowed || usage.LimitReached != nil && *usage.LimitReached {
		return false
	}
	hasKnownMainWindow := false
	for _, label := range []string{"5h", "weekly"} {
		window := doctorOpenAIMainWindow(usage, label)
		if window == nil || window.UsedPercent == nil {
			continue
		}
		hasKnownMainWindow = true
		if *window.UsedPercent >= 100 {
			return false
		}
	}
	return hasKnownMainWindow
}

func doctorOpenAIMainWindows(usage quotamodel.Usage) string {
	parts := make([]string, 0, 2)
	for _, window := range []*quotamodel.Window{usage.Primary, usage.Secondary} {
		if window == nil {
			continue
		}
		label := doctorQuotaWindowLabel(window.LimitWindowSeconds)
		reset := doctorQuotaResetTime(window.ResetAt)
		if window.UsedPercent == nil {
			parts = append(parts, fmt.Sprintf("%s: usage unknown, resets %s", label, reset))
			continue
		}
		remaining := 100 - *window.UsedPercent
		remaining = max(int64(0), min(int64(100), remaining))
		parts = append(parts, fmt.Sprintf("%s: %d%% left (%d%% used), resets %s", label, remaining, *window.UsedPercent, reset))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " | ")
}

func doctorOpenAIBlockedLimits(usage quotamodel.Usage) []string {
	blocked := make([]string, 0, 2)
	for _, label := range []string{"5h", "weekly"} {
		window := doctorOpenAIMainWindow(usage, label)
		if window == nil {
			continue
		}
		switch {
		case window.UsedPercent == nil:
			blocked = append(blocked, label+" quota unknown")
		case *window.UsedPercent >= 100:
			blocked = append(blocked, fmt.Sprintf("%s exhausted until %s", label, doctorQuotaResetTime(window.ResetAt)))
		}
	}
	if len(blocked) > 0 {
		return blocked
	}
	if !doctorOpenAIRateLimitPresent(usage) {
		return []string{"5h quota unavailable", "weekly quota unavailable"}
	}
	return []string{"quota unavailable"}
}

func doctorOpenAIRateLimitPresent(usage quotamodel.Usage) bool {
	return usage.RateLimitPresent || usage.Primary != nil || usage.Secondary != nil ||
		usage.Allowed != nil || usage.LimitReached != nil
}

func doctorOpenAIMainWindow(usage quotamodel.Usage, label string) *quotamodel.Window {
	for _, window := range []*quotamodel.Window{usage.Primary, usage.Secondary} {
		if window != nil && doctorQuotaWindowLabel(window.LimitWindowSeconds) == label {
			return window
		}
	}
	return nil
}

func doctorQuotaWindowLabel(seconds *int64) string {
	if seconds == nil {
		return "usage"
	}
	switch {
	case *seconds >= 17_700 && *seconds <= 18_300:
		return "5h"
	case *seconds >= 601_200 && *seconds <= 608_400:
		return "weekly"
	case *seconds >= 2_505_600 && *seconds <= 2_678_400:
		return "monthly"
	default:
		return fmt.Sprintf("%ds", *seconds)
	}
}

func doctorQuotaResetTime(epoch *int64) string {
	if epoch == nil {
		return "-"
	}
	return time.Unix(*epoch, 0).In(time.Local).Format("2006-01-02 15:04")
}
