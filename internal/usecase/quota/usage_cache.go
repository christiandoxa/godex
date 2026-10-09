package quota

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const (
	usageCacheFreshness = 5 * time.Minute
	maxCachedUsages     = 1024
)

type usageSnapshot struct {
	checkedAt time.Time
	usage     quotamodel.Usage
}

func (status *Status) cachedAvailabilityUsage(ctx context.Context, accountID, home string, selection *quotamodel.Selection) (quotamodel.Usage, quotamodel.Source, error) {
	return status.cachedAvailabilityUsageAtPolicy(ctx, accountID, home, selection, "", false)
}

func (status *Status) cachedAvailabilityUsageAtPolicy(
	ctx context.Context,
	accountID, home string,
	selection *quotamodel.Selection,
	baseURL string,
	noProxy bool,
) (quotamodel.Usage, quotamodel.Source, error) {
	now := status.now()
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		if usage, ok := status.cachedLiveUsage(home, now); ok {
			status.scheduleCachedProbeRefresh(ctx, accountID, home, now)
			return usage, quotamodel.SourceLive, nil
		}
	}
	// Cache hits stay local.
	// Only misses consume the bounded probe gate.
	// Provider errors still release it.
	var usage quotamodel.Usage
	var err error
	switch {
	case baseURL != "" || noProxy:
		if policy, ok := status.usage.(policyUsageGateway); ok {
			usage, err = status.fetchProbe(ctx, func() (quotamodel.Usage, error) {
				return policy.FetchAtPolicy(ctx, home, baseURL, noProxy)
			})
		} else if baseURL != "" && !noProxy {
			if override, ok := status.usage.(overrideUsageGateway); ok {
				usage, err = status.fetchProbe(ctx, func() (quotamodel.Usage, error) {
					return override.FetchAt(ctx, home, baseURL)
				})
			} else {
				err = errors.New("quota base URL override is not supported")
			}
		} else {
			err = statusNoProxyUnsupported()
		}
	default:
		usage, err = status.fetchProbe(ctx, func() (quotamodel.Usage, error) {
			return status.usage.Fetch(ctx, home)
		})
	}
	if err == nil {
		if baseURL == "" {
			status.storeUsage(home, usage, now)
		}
		// Prodex persists a successful startup probe even when the caller
		// supplied a loopback or test upstream override. Keep that durable
		// snapshot separate from the default live cache so an override cannot
		// silently become the next request's transport.
		status.storeUsageSnapshot(ctx, accountID, usage, now)
		return usage, quotamodel.SourceLive, nil
	}
	if snapshot, ok := status.cachedPersistedSnapshot(ctx, accountID, now); ok {
		usage := usageFromSnapshot(snapshot)
		if selection == nil || persistedSnapshotSupportsSelection(usage, *selection, now) {
			return usage, quotamodel.SourcePersistedSnapshot, nil
		}
	}
	return quotamodel.Usage{}, quotamodel.SourceUnknown, err
}

func statusNoProxyUnsupported() error {
	return errors.New("quota --no-proxy requires policy-aware usage transport")
}

func (status *Status) cachedLiveUsage(home string, now time.Time) (quotamodel.Usage, bool) {
	status.usageMu.Lock()
	defer status.usageMu.Unlock()
	snapshot, ok := status.usageCache[home]
	return snapshot.usage, ok && now.Sub(snapshot.checkedAt) >= 0 && now.Sub(snapshot.checkedAt) < usageCacheFreshness
}

func (status *Status) cachedUsageWithSource(ctx context.Context, accountID, home string, now time.Time, selection *quotamodel.Selection) (quotamodel.Usage, quotamodel.Source, bool) {
	if usage, ok := status.cachedLiveUsage(home, now); ok {
		return usage, quotamodel.SourceLive, true
	}
	if snapshot, ok := status.cachedPersistedSnapshot(ctx, accountID, now); ok {
		usage := usageFromSnapshot(snapshot)
		if selection == nil || persistedSnapshotSupportsSelection(usage, *selection, now) {
			return usage, quotamodel.SourcePersistedSnapshot, true
		}
	}
	return quotamodel.Usage{}, quotamodel.SourceUnknown, false
}

func (status *Status) cachedPersistedSnapshot(ctx context.Context, accountID string, now time.Time) (quotamodel.UsageSnapshot, bool) {
	if strings.TrimSpace(accountID) == "" {
		return quotamodel.UsageSnapshot{}, false
	}
	status.usageMu.Lock()
	store := status.snapshots
	if store == nil {
		status.usageMu.Unlock()
		return quotamodel.UsageSnapshot{}, false
	}
	if snapshot, ok := status.snapshotCache[accountID]; ok {
		status.usageMu.Unlock()
		return snapshot, usageSnapshotUsable(snapshot, now)
	}
	status.usageMu.Unlock()
	snapshot, ok, err := store.Load(ctx, accountID)
	if err != nil || !ok {
		return quotamodel.UsageSnapshot{}, false
	}
	status.usageMu.Lock()
	if status.snapshotCache == nil {
		status.snapshotCache = make(map[string]quotamodel.UsageSnapshot)
	}
	status.snapshotCache[accountID] = snapshot
	status.usageMu.Unlock()
	return snapshot, usageSnapshotUsable(snapshot, now)
}

func (status *Status) storeUsage(home string, usage quotamodel.Usage, now time.Time) {
	status.usageMu.Lock()
	defer status.usageMu.Unlock()
	if status.usageCache == nil {
		status.usageCache = make(map[string]usageSnapshot)
	}
	if len(status.usageCache) >= maxCachedUsages {
		oldestHome := ""
		var oldest time.Time
		for candidate, snapshot := range status.usageCache {
			if oldestHome == "" || snapshot.checkedAt.Before(oldest) {
				oldestHome, oldest = candidate, snapshot.checkedAt
			}
		}
		delete(status.usageCache, oldestHome)
	}
	status.usageCache[home] = usageSnapshot{checkedAt: now, usage: usage}
}

func (status *Status) storeUsageSnapshot(ctx context.Context, accountID string, usage quotamodel.Usage, now time.Time) {
	if strings.TrimSpace(accountID) == "" {
		return
	}
	status.usageMu.Lock()
	store := status.snapshots
	if store == nil {
		status.usageMu.Unlock()
		return
	}
	status.usageMu.Unlock()
	snapshot := usageSnapshotFromUsage(usage, now)
	status.usageMu.Lock()
	if status.snapshotCache == nil {
		status.snapshotCache = make(map[string]quotamodel.UsageSnapshot)
	}
	status.snapshotCache[accountID] = snapshot
	status.usageMu.Unlock()
	_ = store.Save(context.WithoutCancel(ctx), accountID, snapshot)
}

func usageSnapshotFromUsage(usage quotamodel.Usage, now time.Time) quotamodel.UsageSnapshot {
	plan := strings.TrimSpace(usage.PlanType)
	var planType *string
	if plan != "" {
		planType = &plan
	}
	fivePresent := usage.Primary != nil && usage.Primary.UsedPercent != nil
	weeklyPresent := usage.Secondary != nil && usage.Secondary.UsedPercent != nil
	fiveStatus, fiveRemaining, fiveReset := usageWindowSnapshot(usage.Primary)
	weeklyStatus, weeklyRemaining, weeklyReset := usageWindowSnapshot(usage.Secondary)
	if fivePresent != weeklyPresent {
		if !fivePresent {
			fiveStatus, fiveRemaining, fiveReset = quotamodel.WindowReady, 100, math.MaxInt64
		} else {
			weeklyStatus, weeklyRemaining, weeklyReset = quotamodel.WindowReady, 100, math.MaxInt64
		}
	}
	return quotamodel.UsageSnapshot{
		CheckedAt: now.Unix(), PlanType: planType,
		FiveHourStatus: fiveStatus, FiveHourRemainingPercent: fiveRemaining, FiveHourResetAt: fiveReset,
		WeeklyStatus: weeklyStatus, WeeklyRemainingPercent: weeklyRemaining, WeeklyResetAt: weeklyReset,
	}
}

func usageWindowSnapshot(window *quotamodel.Window) (quotamodel.WindowStatus, int64, int64) {
	if window == nil || window.UsedPercent == nil {
		return quotamodel.WindowUnknown, 0, math.MaxInt64
	}
	remaining := max(int64(0), min(int64(100), 100-*window.UsedPercent))
	resetAt := int64(math.MaxInt64)
	if window.ResetAt != nil {
		resetAt = *window.ResetAt
	}
	switch {
	case remaining == 0:
		return quotamodel.WindowExhausted, remaining, resetAt
	case remaining <= 5:
		return quotamodel.WindowCritical, remaining, resetAt
	case remaining <= 15:
		return quotamodel.WindowThin, remaining, resetAt
	default:
		return quotamodel.WindowReady, remaining, resetAt
	}
}

func persistedSnapshotSupportsSelection(usage quotamodel.Usage, selection quotamodel.Selection, now time.Time) bool {
	if !isLunaModel(selection.RequestedModel) {
		return true
	}
	return quotaPairReady(usage.Allowed, usage.LimitReached, usage.Primary, usage.Secondary, now)
}

func usageSnapshotUsable(snapshot quotamodel.UsageSnapshot, now time.Time) bool {
	unix := now.Unix()
	active := (snapshot.FiveHourStatus == quotamodel.WindowExhausted && snapshot.FiveHourResetAt != math.MaxInt64 && snapshot.FiveHourResetAt > unix) ||
		(snapshot.WeeklyStatus == quotamodel.WindowExhausted && snapshot.WeeklyResetAt != math.MaxInt64 && snapshot.WeeklyResetAt > unix)
	if active {
		return true
	}
	expired := (snapshot.FiveHourStatus == quotamodel.WindowExhausted && snapshot.FiveHourResetAt != math.MaxInt64 && snapshot.FiveHourResetAt <= unix) ||
		(snapshot.WeeklyStatus == quotamodel.WindowExhausted && snapshot.WeeklyResetAt != math.MaxInt64 && snapshot.WeeklyResetAt <= unix)
	if expired {
		return false
	}
	return unix-snapshot.CheckedAt <= int64((30*time.Minute)/time.Second)
}

func usageFromSnapshot(snapshot quotamodel.UsageSnapshot) quotamodel.Usage {
	usage := quotamodel.Usage{RateLimitPresent: true}
	if snapshot.PlanType != nil {
		usage.PlanType = *snapshot.PlanType
	}
	usage.Primary = usageWindowFromSnapshot(snapshot.FiveHourStatus, snapshot.FiveHourRemainingPercent, snapshot.FiveHourResetAt, 18_000)
	usage.Secondary = usageWindowFromSnapshot(snapshot.WeeklyStatus, snapshot.WeeklyRemainingPercent, snapshot.WeeklyResetAt, 604_800)
	return usage
}

func usageWindowFromSnapshot(status quotamodel.WindowStatus, remaining, resetAt, windowSeconds int64) *quotamodel.Window {
	if status == quotamodel.WindowUnknown {
		return nil
	}
	used := max(int64(0), min(int64(100), 100-remaining))
	window := &quotamodel.Window{UsedPercent: &used, LimitWindowSeconds: &windowSeconds}
	if resetAt != math.MaxInt64 {
		window.ResetAt = &resetAt
	}
	return window
}

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
