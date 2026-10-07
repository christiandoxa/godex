package routing

import (
	"context"
	"strconv"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func (router *Router) recordSelectionMarker(
	ctx context.Context,
	kind string,
	account proxymodel.Account,
	selection quotamodel.Selection,
) {
	if router == nil || router.activity == nil {
		return
	}
	fields := map[string]string{
		"profile": account.ID,
		"route":   routeHealthRoute(selection.RouteKind),
	}
	if state, ok := router.cachedQuotaCheck(account.ID, selection, router.now()); ok && state.pressure.Known {
		fields["five_hour_remaining"] = strconv.FormatInt(state.pressure.FiveHourRemaining, 10)
		fields["weekly_remaining"] = strconv.FormatInt(state.pressure.WeeklyRemaining, 10)
	}
	router.recordRuntimeMarker(ctx, runtimemodel.Event{
		Kind:      kind,
		AccountID: account.ID,
		Fields:    fields,
	})
}

func (router *Router) recordProfileInflightMarker(accountID string, count int) {
	if router == nil || router.activity == nil || accountID == "" {
		return
	}
	router.recordRuntimeMarker(context.Background(), runtimemodel.Event{
		Kind:      "profile_inflight",
		AccountID: accountID,
		Fields: map[string]string{
			"profile": accountID,
			"count":   strconv.Itoa(max(count, 0)),
		},
	})
}
