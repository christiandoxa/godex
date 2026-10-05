package routing

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type gateway interface {
	Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error)
}
type ActivityRecorder interface {
	Record(context.Context, runtimemodel.Event) error
}

type AutoRedeemer interface {
	Try(context.Context, []proxymodel.Account, string, proxymodel.Request) (string, bool, error)
}

type quotaPreflight interface {
	AvailabilityForRoute(context.Context, accountentity.Account, quotamodel.Selection) (quotamodel.Availability, error)
}

type cachedQuotaPreflight interface {
	CachedAvailabilityForRoute(accountentity.Account, quotamodel.Selection, time.Time) (quotamodel.Availability, bool)
}

type routingStateRepository interface {
	LoadRouteHealth(context.Context, time.Time) ([]routingentity.RouteHealthScore, error)
	AdjustRouteHealth(context.Context, string, string, int, time.Time) (routingentity.RouteHealthScore, error)
	LoadRetryBackoffs(context.Context, time.Time) ([]routingentity.RetryBackoff, error)
	SetRetryBackoff(context.Context, routingentity.RetryBackoff, time.Time) error
	ClearRetryBackoff(context.Context, string, time.Time) error
	LoadTransportBackoffs(context.Context, time.Time) ([]routingentity.TransportBackoff, error)
	SetTransportBackoff(context.Context, routingentity.TransportBackoff, time.Time) error
	ClearTransportBackoff(context.Context, string, string) error
	LoadRouteCircuits(context.Context, time.Time, []routingentity.RouteHealthScore) ([]routingentity.RouteCircuit, error)
	OpenRouteCircuit(context.Context, string, string, uint8, time.Time) (routingentity.RouteCircuit, bool, error)
	ClearRouteCircuit(context.Context, string, string) error
	ReserveRouteCircuitProbe(context.Context, string, string, uint8, time.Time) (routingentity.RouteCircuit, bool, error)
}

type Config struct {
	Accounts                 func(context.Context) ([]proxymodel.Account, error)
	PreferredAccount         string
	Activity                 ActivityRecorder
	QuotaPreflight           quotaPreflight
	RoutingState             routingStateRepository
	Now                      func() time.Time
	Wait                     func(context.Context, time.Duration) error
	ProfileInflightWait      profileInflightWaitFunc
	ProfileInflightHardLimit int
	MaxInspectBytes          int64
	Gateway                  gateway
	Bindings                 bindingRepository
	AutoRedeem               bool
	Redeemer                 AutoRedeemer
}

type Router struct {
	source                    func(context.Context) ([]proxymodel.Account, error)
	gateway                   gateway
	quota                     quotaPreflight
	state                     routingStateRepository
	activity                  ActivityRecorder
	preferred                 string
	now                       func() time.Time
	wait                      func(context.Context, time.Duration) error
	maxInspect                int64
	affinity                  *affinityStore
	mu                        sync.Mutex
	retryBackoffMu            sync.Mutex
	transportMu               sync.Mutex
	routeHealthMu             sync.Mutex
	routeCircuitMu            sync.Mutex
	previousResponseFailureMu sync.Mutex
	cursor                    int
	preferredUsed             bool
	inflight                  map[string]int
	inflightChanged           chan struct{}
	profileInflightHardLimit  int
	profileInflightWait       profileInflightWaitFunc
	quarantine                map[string]quarantineState
	quotaBlocked              map[string]bool
	quotaChecks               map[quotaCheckKey]quotaCheck
	routeHealth               map[routeHealthKey]routingentity.RouteHealthScore
	routeCircuits             map[routeHealthKey]routingentity.RouteCircuit
	transportBackoffs         map[routeHealthKey]routingentity.TransportBackoff
	previousResponseFailures  map[previousResponseFailureKey]routingentity.PreviousResponseFailure
	autoRedeem                bool
	redeemer                  AutoRedeemer
	conversations             map[string]*conversationLock
}

func NewRouter(config Config) (*Router, error) {
	if config.Accounts == nil {
		return nil, errors.New("routing account source is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Wait == nil {
		config.Wait = waitContext
	}
	if config.MaxInspectBytes <= 0 {
		config.MaxInspectBytes = 64 << 10
	}
	if config.ProfileInflightHardLimit <= 0 {
		config.ProfileInflightHardLimit = defaultProfileInflightHardLimit
	}
	if config.ProfileInflightWait == nil {
		config.ProfileInflightWait = waitProfileInflightSignalOrEpoch
	}
	router := &Router{
		source: config.Accounts, gateway: config.Gateway,
		quota:     config.QuotaPreflight,
		state:     config.RoutingState,
		activity:  config.Activity,
		preferred: strings.TrimSpace(config.PreferredAccount),
		now:       config.Now, wait: config.Wait, maxInspect: config.MaxInspectBytes,
		affinity: newAffinityStore(), quarantine: make(map[string]quarantineState),
		inflight: make(map[string]int), inflightChanged: make(chan struct{}),
		profileInflightHardLimit: config.ProfileInflightHardLimit, profileInflightWait: config.ProfileInflightWait,
		quotaBlocked: make(map[string]bool), quotaChecks: make(map[quotaCheckKey]quotaCheck),
		routeHealth:              make(map[routeHealthKey]routingentity.RouteHealthScore),
		routeCircuits:            make(map[routeHealthKey]routingentity.RouteCircuit),
		transportBackoffs:        make(map[routeHealthKey]routingentity.TransportBackoff),
		previousResponseFailures: make(map[previousResponseFailureKey]routingentity.PreviousResponseFailure),
		autoRedeem:               config.AutoRedeem, redeemer: config.Redeemer,
	}
	router.affinity.repository = config.Bindings
	if config.RoutingState != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		now := config.Now()
		scores, err := config.RoutingState.LoadRouteHealth(ctx, now)
		if err != nil {
			return nil, fmt.Errorf("load routing health: %w", err)
		}
		for _, score := range scores {
			router.routeHealth[routeHealthKey{accountID: score.AccountID, route: score.Route}] = score
		}
		circuits, err := config.RoutingState.LoadRouteCircuits(ctx, now, scores)
		if err != nil {
			return nil, fmt.Errorf("load routing circuits: %w", err)
		}
		for _, circuit := range circuits {
			key := routeHealthKey{accountID: circuit.AccountID, route: circuit.Route}
			router.routeCircuits[key] = circuit
		}
		backoffs, err := config.RoutingState.LoadRetryBackoffs(ctx, now)
		if err != nil {
			return nil, fmt.Errorf("load routing retry backoffs: %w", err)
		}
		for _, backoff := range backoffs {
			router.setQuarantine(backoff.AccountID, backoff.Remaining(now), false)
		}
		transportBackoffs, err := config.RoutingState.LoadTransportBackoffs(ctx, now)
		if err != nil {
			return nil, fmt.Errorf("load routing transport backoffs: %w", err)
		}
		for _, backoff := range transportBackoffs {
			router.transportBackoffs[routeHealthKey{accountID: backoff.AccountID, route: backoff.Route}] = backoff
		}
		if err := router.loadPreviousResponseFailures(ctx, now); err != nil {
			return nil, fmt.Errorf("load previous response failures: %w", err)
		}
	}
	return router, nil
}

type Exchange struct {
	Result  proxymodel.Forwarded
	release func()
}

func (exchange *Exchange) Close() error {
	if exchange.release != nil {
		exchange.release()
	}
	return exchange.Result.Response.Body.Close()
}

func (router *Router) Forward(ctx context.Context, request proxymodel.Request) (*Exchange, error) {
	keys := requestRoutingAffinity(request)
	release, err := router.acquireConversation(ctx, keys)
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			release()
		}
	}()

	accounts, err := router.loadAccounts(ctx)
	if err != nil {
		return nil, err
	}
	owner, durableRelease, err := router.resolveOwner(ctx, keys, request.QuotaSelection)
	if durableRelease != nil {
		defer durableRelease()
	}
	if err != nil {
		return nil, err
	}
	restorePreviousResponseTurnState(ctx, &request, accounts, owner, &keys, router.affinity, router.now())
	result, err := router.routeRequest(ctx, request, accounts, owner, &keys)
	if err != nil {
		return nil, err
	}
	if err := router.bindSuccessfulResponse(ctx, &result, accounts, keys, request.WebSocketMessage); err != nil {
		return nil, err
	}
	transferred = true
	return &Exchange{Result: result, release: release}, nil
}

func (router *Router) loadAccounts(ctx context.Context) ([]proxymodel.Account, error) {
	accounts, err := router.source(ctx)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: 503, Message: "cannot load managed accounts"}
	}
	return sortRuntimeAccounts(accounts), nil
}

func (router *Router) resolveOwner(
	ctx context.Context,
	keys affinityKeys,
	selection quotamodel.Selection,
) (string, func() error, error) {
	owner, err := router.affinity.owner(ctx, keys, router.now())
	if err != nil {
		return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
	}
	var durableRelease func() error
	if owner == "" && router.affinity.repository != nil && stableConversation(keys) {
		durableRelease, err = router.affinity.repository.AcquireConversation(ctx)
		if err != nil {
			return "", nil, err
		}
		owner, err = router.affinity.owner(ctx, keys, router.now())
		if err != nil {
			_ = durableRelease()
			return "", nil, err
		}
	}
	if owner == "" && opaqueContinuation(keys) &&
		(keys.previous == "" || !router.hasPreviousResponseFailure(keys.previous, selection)) {
		if durableRelease != nil {
			_ = durableRelease()
		}
		return "", nil, &proxymodel.Error{StatusCode: 409, Message: "continuation owner is unknown; continuity was preserved"}
	}
	return owner, durableRelease, nil
}

func stableConversation(keys affinityKeys) bool {
	return keys.thread != "" || keys.session != ""
}

func opaqueContinuation(keys affinityKeys) bool {
	return keys.previous != "" || keys.turn != ""
}

func (router *Router) routeRequest(ctx context.Context, request proxymodel.Request, accounts []proxymodel.Account, owner string, keys *affinityKeys) (proxymodel.Forwarded, error) {
	if owner != "" {
		return router.forwardBound(ctx, request, accounts, owner, keys)
	}
	return router.forwardFresh(ctx, request, accounts)
}

func (router *Router) bindSuccessfulResponse(
	ctx context.Context,
	result *proxymodel.Forwarded,
	accounts []proxymodel.Account,
	keys affinityKeys,
	websocketMessage bool,
) error {
	if result.Response.PrecommitFailure != nil && result.Response.PrecommitFailure.StaleContinuation {
		result.Failed = true
		return nil
	}
	if result.Failed || result.Response.StatusCode >= 400 {
		return nil
	}
	router.clearPreviousResponseFailures(ctx, result.AccountID, keys.previous)
	if websocketMessage {
		if result.Response.WebSocketResponseID != "" {
			keys.previous = result.Response.WebSocketResponseID
			if turnState := responseTurnStateValue(result.Response); turnState != "" {
				keys.turn = turnState
			}
		}
		now := router.now()
		if err := router.affinity.remember(ctx, result.AccountID, keys, now); err != nil {
			return err
		}
		router.affinity.rememberResponseTurnStateForHome(
			ctx, keys.previous, result.AccountID, responseTurnStateHome(accounts, result.AccountID), keys.turn, now,
		)
		return nil
	}
	if result.Response.StatusCode == http.StatusSwitchingProtocols {
		keys.previous = result.Response.WebSocketResponseID
		if turnState := responseTurnStateValue(result.Response); turnState != "" {
			keys.turn = turnState
		}
		now := router.now()
		if err := router.affinity.remember(ctx, result.AccountID, keys, now); err != nil {
			return err
		}
		router.affinity.rememberResponseTurnStateForHome(
			ctx, keys.previous, result.AccountID, responseTurnStateHome(accounts, result.AccountID), keys.turn, now,
		)
		return nil
	}
	stream := strings.Contains(strings.ToLower(result.Response.Header.Get("Content-Type")), "text/event-stream")
	if result.Prefix == nil && !stream {
		prefix, _, err := inspectResponse(result.Response.Body, router.maxInspect)
		if err != nil {
			result.Response.Body.Close()
			return &proxymodel.Error{StatusCode: 502, Message: "upstream response failed before commitment"}
		}
		result.Prefix = prefix
	}
	if err := router.affinity.remember(ctx, result.AccountID, keys, router.now()); err != nil {
		result.Response.Body.Close()
		return err
	}
	if err := router.Observe(ctx, result.AccountID, result.Response.Header, result.Prefix, stream); err != nil {
		result.Response.Body.Close()
		return err
	}
	return nil
}

func (router *Router) Observe(ctx context.Context, accountID string, headers http.Header, body []byte, stream bool) error {
	keys := responseAffinity(headers, body, stream)
	now := router.now()
	if err := router.affinity.remember(ctx, accountID, keys, now); err != nil {
		return err
	}
	router.affinity.rememberResponseTurnState(keys.previous, accountID, keys.turn, now)
	return nil
}

func (router *Router) execute(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	release := router.beginRequestInFlight(account.ID, request.QuotaSelection)
	response, err := router.executeAccount(ctx, request, account)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		release()
		return nil, err
	}
	if response == nil || response.Body == nil {
		release()
		return response, nil
	}
	body := &inFlightBody{ReadCloser: response.Body, release: release}
	if duplex, ok := response.Body.(io.ReadWriteCloser); ok {
		response.Body = &inFlightDuplexBody{inFlightBody: body, duplex: duplex}
	} else {
		response.Body = body
	}
	return response, nil
}

func (router *Router) executeAccount(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	if router.gateway == nil {
		return nil, errors.New("routing gateway is not configured")
	}
	if strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") {
		return router.executeWebSocketRequest(ctx, request, account)
	}
	if externalProviderKind(account.Provider.Kind) {
		if strings.EqualFold(strings.TrimSpace(account.Provider.Kind), "gemini") {
			return router.executeGeminiModelFallback(ctx, request, account)
		}
		return router.gateway.Execute(ctx, request, account)
	}
	for reload := 0; reload < 2; reload++ {
		response, err := router.gateway.Execute(ctx, request, account)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized || reload == 1 {
			return response, nil
		}
		response.Body.Close()
	}
	return nil, errors.New("authentication retry failed")
}

func (router *Router) Close() {
	if closer, ok := router.gateway.(interface{ Close() }); ok {
		closer.Close()
	}
}

func (router *Router) recordRuntimeMarker(ctx context.Context, event runtimemodel.Event) {
	if router == nil || router.activity == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	} else {
		ctx = context.WithoutCancel(ctx)
	}
	_ = router.activity.Record(ctx, event)
}
