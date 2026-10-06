package routing

import (
	"context"
	"io"
	"strconv"
	"sync"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func (router *Router) recordRouteLatencyObservation(
	ctx context.Context,
	accountID string,
	selection quotamodel.Selection,
	stage string,
	elapsed time.Duration,
) {
	if router == nil || router.now == nil || accountID == "" || routeHealthRoute(selection.RouteKind) == "" {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	elapsedMS := uint64(elapsed / time.Millisecond)
	observed := healthLatencyPenalty(elapsedMS, selection.RouteKind, stage)
	updated := router.mutateRouteMemory(ctx, accountID, selection, routingentity.RouteMemoryPerformance, func(score routingentity.RouteMemoryScore) routingentity.RouteMemoryScore {
		current := score.Effective(router.now())
		score.Score = healthLatencyNextScore(current, observed)
		return score
	})
	router.recordRouteLatencyMarker(ctx, accountID, selection, stage, elapsedMS, updated.Score)
}

func (router *Router) recordRouteLatencyFailure(
	ctx context.Context,
	accountID string,
	selection quotamodel.Selection,
	stage string,
) {
	if router == nil || router.now == nil || accountID == "" || routeHealthRoute(selection.RouteKind) == "" {
		return
	}
	updated := router.mutateRouteMemory(ctx, accountID, selection, routingentity.RouteMemoryPerformance, func(score routingentity.RouteMemoryScore) routingentity.RouteMemoryScore {
		current := score.Effective(router.now())
		score.Score = healthLatencyFailureNextScore(current)
		return score
	})
	router.recordRouteLatencyMarker(ctx, accountID, selection, stage, 0, updated.Score)
}

func (router *Router) recordRouteLatencyMarker(
	ctx context.Context,
	accountID string,
	selection quotamodel.Selection,
	stage string,
	elapsedMS uint64,
	score uint8,
) {
	fields := map[string]string{
		"profile": accountID,
		"route":   routeHealthRoute(selection.RouteKind),
		"score":   strconv.Itoa(int(score)),
		"reason":  stage,
	}
	if elapsedMS > 0 {
		fields["latency_ms"] = strconv.FormatUint(elapsedMS, 10)
	}
	router.recordRuntimeMarker(ctx, runtimemodel.Event{Kind: "profile_latency", Fields: fields})
}

func (router *Router) observedGatewayAttempt(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	execute func() (*proxymodel.Response, error),
) (*proxymodel.Response, error) {
	if router == nil || router.now == nil {
		return execute()
	}
	started := router.now()
	response, err := execute()
	if err != nil {
		return response, err
	}
	router.recordRouteLatencyObservation(
		context.Background(), account.ID, request.QuotaSelection, "connect", router.elapsedSince(started),
	)
	return response, nil
}

func (router *Router) observedWebSocketConnectAttempt(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	execute func() (*proxymodel.Response, error),
) (*proxymodel.Response, error) {
	if router == nil || router.now == nil {
		return execute()
	}
	started := router.now()
	response, err := execute()
	if err != nil {
		return response, err
	}
	if response != nil && response.StatusCode == 101 {
		router.recordRouteLatencyObservation(
			context.Background(), account.ID, request.QuotaSelection, "connect", router.elapsedSince(started),
		)
	}
	return response, nil
}

func (router *Router) observedWebSocketMessageAttempt(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	execute func() (*proxymodel.Response, error),
) (*proxymodel.Response, error) {
	response, err := execute()
	if router != nil && router.now != nil {
		router.observeWebSocketMessageConnect(request, account, response, err)
	}
	return response, err
}

func (router *Router) observeWebSocketMessageConnect(
	request proxymodel.Request,
	account proxymodel.Account,
	response *proxymodel.Response,
	err error,
) {
	if err != nil {
		return
	}
	if response == nil || response.UpstreamConnectDuration <= 0 {
		return
	}
	router.recordRouteLatencyObservation(
		context.Background(), account.ID, request.QuotaSelection, "connect", response.UpstreamConnectDuration,
	)
}

func (router *Router) elapsedSince(started time.Time) time.Duration {
	elapsed := router.now().Sub(started)
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func (router *Router) wrapResponsesStreamLatency(
	accountID string,
	selection quotamodel.Selection,
	response *proxymodel.Response,
	prefixPresent bool,
) {
	if router == nil || router.now == nil || response == nil || response.Body == nil || selection.RouteKind != quotamodel.RouteKindResponses {
		return
	}
	body := &routeLatencyBody{
		ReadCloser: response.Body,
		router:     router,
		accountID:  accountID,
		selection:  selection,
		started:    router.now(),
	}
	if prefixPresent {
		body.first.Do(func() {
			router.recordRouteLatencyObservation(
				context.Background(), accountID, selection, "ttfb", 0,
			)
		})
	}
	response.Body = body
}

type routeLatencyBody struct {
	io.ReadCloser
	router    *Router
	accountID string
	selection quotamodel.Selection
	started   time.Time
	first     sync.Once
	complete  sync.Once
	failure   sync.Once
}

func (body *routeLatencyBody) Read(buffer []byte) (int, error) {
	count, err := body.ReadCloser.Read(buffer)
	if count > 0 {
		body.first.Do(func() {
			body.router.recordRouteLatencyObservation(
				context.Background(), body.accountID, body.selection, "ttfb", body.router.elapsedSince(body.started),
			)
		})
	}
	if err == io.EOF {
		body.complete.Do(func() {
			body.router.recordRouteLatencyObservation(
				context.Background(), body.accountID, body.selection, "stream_complete", body.router.elapsedSince(body.started),
			)
		})
	} else if err != nil {
		body.failure.Do(func() {
			body.router.recordRouteLatencyFailure(context.Background(), body.accountID, body.selection, "stream_read_error")
		})
	}
	return count, err
}
