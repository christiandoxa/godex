package quota

import (
	"context"
	"strings"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (status *Status) Availability(ctx context.Context, account accountentity.Account) (quotamodel.Availability, error) {
	return status.AvailabilityWithPolicy(ctx, account, false)
}

func (status *Status) AvailabilityWithPolicy(
	ctx context.Context,
	account accountentity.Account,
	noProxy bool,
) (quotamodel.Availability, error) {
	return status.AvailabilityAtPolicy(ctx, account, "", noProxy)
}

func (status *Status) AvailabilityAtPolicy(
	ctx context.Context,
	account accountentity.Account,
	baseURL string,
	noProxy bool,
) (quotamodel.Availability, error) {
	if !account.Enabled {
		return quotamodel.Availability{}, nil
	}
	return status.availabilityAtPolicyForHome(ctx, account.ID, status.accounts.CodexHome(account.ID), baseURL, noProxy)
}

func (status *Status) availabilityAtPolicyForHome(
	ctx context.Context,
	accountID, home, baseURL string,
	noProxy bool,
) (quotamodel.Availability, error) {
	usage, source, err := status.cachedAvailabilityUsageAtPolicy(ctx, accountID, home, nil, baseURL, noProxy)
	if err != nil {
		return quotamodel.Availability{}, err
	}
	return availabilityFromUsage(usage, source, status.now()), nil
}

func availabilityFromUsage(
	usage quotamodel.Usage,
	source quotamodel.Source,
	now time.Time,
) quotamodel.Availability {
	ready := quotaState(quotamodel.Report{Enabled: true, Usage: usage}, now) != "exhausted"
	if ready {
		return quotamodel.Availability{Ready: true, Source: source}
	}
	retry := time.Time{}
	for _, window := range []*quotamodel.Window{usage.Primary, usage.Secondary} {
		if window == nil || window.ResetAt == nil || *window.ResetAt <= now.Unix() {
			continue
		}
		if window.UsedPercent != nil && *window.UsedPercent < 100 {
			continue
		}
		reset := time.Unix(*window.ResetAt, 0)
		if reset.After(retry) {
			retry = reset
		}
	}
	if retry.IsZero() {
		retry = now.Add(time.Minute)
	}
	return quotamodel.Availability{RetryAt: retry, Source: source}
}

func (status *Status) AvailabilityForRoute(
	ctx context.Context,
	account accountentity.Account,
	selection quotamodel.Selection,
) (quotamodel.Availability, error) {
	if !account.Enabled {
		return quotamodel.Availability{}, nil
	}
	usage, source, err := status.cachedAvailabilityUsage(ctx, account.ID, status.accounts.CodexHome(account.ID), &selection)
	if err != nil {
		return quotamodel.Availability{}, err
	}
	availability := routeAvailability(usage, selection, status.now())
	availability.Source = source
	return availability, nil
}

func (status *Status) CachedAvailabilityForRoute(
	account accountentity.Account,
	selection quotamodel.Selection,
	now time.Time,
) (quotamodel.Availability, bool) {
	if !account.Enabled {
		return quotamodel.Availability{}, false
	}
	usage, source, ok := status.cachedUsageWithSource(context.Background(), account.ID, status.accounts.CodexHome(account.ID), now, &selection)
	if !ok {
		return quotamodel.Availability{}, false
	}
	availability := routeAvailability(usage, selection, now)
	availability.Source = source
	return availability, true
}

func routeAvailability(usage quotamodel.Usage, selection quotamodel.Selection, now time.Time) quotamodel.Availability {
	availability := quotamodel.Availability{Pressure: quotaPressure(usage, selection, now)}
	blocked, retry := routeQuotaBlocked(usage, selection, now)
	if !blocked {
		availability.Ready = true
		return availability
	}
	if retry.IsZero() {
		retry = now.Add(time.Minute)
	}
	availability.RetryAt = retry
	return availability
}

func routeQuotaBlocked(usage quotamodel.Usage, selection quotamodel.Selection, now time.Time) (bool, time.Time) {
	if isRetiredQuotaModel(selection.RequestedModel) {
		return true, time.Time{}
	}
	primary, secondary, allowed, limitReached := quotaWindowsForModel(usage, selection, now)
	windows := routeQuotaWindows(primary, secondary, selection.RouteKind)
	return quotaPairBlocked(allowed, limitReached, windows, now)
}

func quotaWindowsForModel(
	usage quotamodel.Usage,
	selection quotamodel.Selection,
	now time.Time,
) (*quotamodel.Window, *quotamodel.Window, *bool, *bool) {
	primary, secondary, allowed, limitReached := usage.Primary, usage.Secondary, usage.Allowed, usage.LimitReached
	if !isLunaModel(selection.RequestedModel) || quotaPairReady(allowed, limitReached, primary, secondary, now) {
		return primary, secondary, allowed, limitReached
	}
	for _, limit := range usage.AdditionalRateLimits {
		if lunaReserve(limit) && quotaPairReady(limit.Allowed, limit.LimitReached, limit.Primary, limit.Secondary, now) {
			return limit.Primary, limit.Secondary, limit.Allowed, limit.LimitReached
		}
	}
	return primary, secondary, allowed, limitReached
}

func quotaPairReady(allowed, limitReached *bool, primary, secondary *quotamodel.Window, now time.Time) bool {
	if !hasQuotaWindow(primary) && !hasQuotaWindow(secondary) {
		return false
	}
	blocked, _ := quotaPairBlocked(allowed, limitReached, []*quotamodel.Window{primary, secondary}, now)
	return !blocked
}

func hasQuotaWindow(window *quotamodel.Window) bool {
	return window != nil && window.UsedPercent != nil
}

func routeQuotaWindows(primary, secondary *quotamodel.Window, route quotamodel.RouteKind) []*quotamodel.Window {
	// Runtime lanes block on active five-hour exhaustion; weekly pressure alone is not admission.
	switch route {
	case quotamodel.RouteKindResponses, quotamodel.RouteKindCompact, quotamodel.RouteKindWebSocket:
		return []*quotamodel.Window{primary}
	default:
		return []*quotamodel.Window{primary, secondary}
	}
}

func quotaPairBlocked(allowed, limitReached *bool, windows []*quotamodel.Window, now time.Time) (bool, time.Time) {
	if (allowed != nil && !*allowed) || (limitReached != nil && *limitReached) {
		return true, latestWindowReset(windows, now)
	}
	blocked := false
	var retry time.Time
	for _, window := range windows {
		if window == nil || window.UsedPercent == nil || *window.UsedPercent < 100 {
			continue
		}
		if window.ResetAt == nil || *window.ResetAt > now.Unix() {
			blocked = true
			if window.ResetAt != nil && time.Unix(*window.ResetAt, 0).After(retry) {
				retry = time.Unix(*window.ResetAt, 0)
			}
		}
	}
	return blocked, retry
}

func latestWindowReset(windows []*quotamodel.Window, now time.Time) time.Time {
	var retry time.Time
	for _, window := range windows {
		if window == nil || window.ResetAt == nil || *window.ResetAt <= now.Unix() {
			continue
		}
		if window.UsedPercent != nil && *window.UsedPercent < 100 {
			continue
		}
		reset := time.Unix(*window.ResetAt, 0)
		if reset.After(retry) {
			retry = reset
		}
	}
	return retry
}

func lunaReserve(limit quotamodel.AdditionalRateLimit) bool {
	if !isLunaModel(limit.NormalModelSlug) {
		return false
	}
	for _, value := range []string{limit.LimitID, limit.LimitName, limit.MeteredFeature} {
		identifier := normalizeQuotaModel(value)
		if identifier == "gptreserve" || strings.Contains(identifier, "luna") && strings.Contains(identifier, "reserve") {
			return true
		}
	}
	return false
}

func isLunaModel(model string) bool {
	switch normalizeQuotaModel(model) {
	case "luna", "gpt56luna":
		return true
	default:
		return false
	}
}

func isRetiredQuotaModel(model string) bool {
	switch normalizeQuotaModel(model) {
	case "spark", "gpt53codexspark", "gpt53spark":
		return true
	default:
		return false
	}
}

func normalizeQuotaModel(model string) string {
	return strings.Map(func(char rune) rune {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			return char
		}
		return -1
	}, strings.ToLower(model))
}
