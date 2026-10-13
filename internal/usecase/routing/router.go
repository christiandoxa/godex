package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	SetRouteHealth(context.Context, string, string, uint8, time.Time) (routingentity.RouteHealthScore, error)
	LoadRouteMemory(context.Context, time.Time) ([]routingentity.RouteMemoryScore, error)
	MutateRouteMemory(context.Context, string, string, string, time.Time, func(routingentity.RouteMemoryScore) routingentity.RouteMemoryScore) (routingentity.RouteMemoryScore, error)
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
	// PrecommitWatchdogBudget overrides the Prodex 0.437.8 five-minute
	// request-scoped default, primarily for source-pinned fixture tests.
	PrecommitWatchdogBudget time.Duration
	SelectionSequenceSeed   uint64
	MaxInspectBytes         int64
	Gateway                 gateway
	Bindings                bindingRepository
	AutoRedeem              bool
	Redeemer                AutoRedeemer
}

type Router struct {
	source                                func(context.Context) ([]proxymodel.Account, error)
	gateway                               gateway
	quota                                 quotaPreflight
	state                                 routingStateRepository
	activity                              ActivityRecorder
	preferred                             string
	now                                   func() time.Time
	wait                                  func(context.Context, time.Duration) error
	maxInspect                            int64
	affinity                              *affinityStore
	mu                                    sync.Mutex
	retryBackoffMu                        sync.Mutex
	transportMu                           sync.Mutex
	routeHealthMu                         sync.Mutex
	routeMemoryMu                         sync.Mutex
	routeCircuitMu                        sync.Mutex
	previousResponseFailureMu             sync.Mutex
	promptCacheMu                         sync.Mutex
	selection                             selectionState
	persistenceEnabled                    atomic.Bool
	cursor                                int
	preferredUsed                         bool
	inflight                              map[string]int
	inflightChanged                       chan struct{}
	profileInflightHardLimit              int
	profileInflightWait                   profileInflightWaitFunc
	precommitWatchdogBudget               time.Duration
	profileInflightAdmissionsTotal        uint64
	profileInflightReleasesTotal          uint64
	profileInflightReleaseUnderflowsTotal uint64
	ephemeralCredentialIDs                map[string]struct{}
	quarantine                            map[string]quarantineState
	quotaBlocked                          map[string]bool
	quotaChecks                           map[quotaCheckKey]quotaCheck
	routeHealth                           map[routeHealthKey]routingentity.RouteHealthScore
	routeMemory                           map[routeMemoryKey]routingentity.RouteMemoryScore
	routeCircuits                         map[routeHealthKey]routingentity.RouteCircuit
	transportBackoffs                     map[routeHealthKey]routingentity.TransportBackoff
	accountHomes                          map[string]string
	previousResponseFailures              map[previousResponseFailureKey]routingentity.PreviousResponseFailure
	promptCacheBindings                   map[string]promptCacheBinding
	autoRedeem                            bool
	redeemer                              AutoRedeemer
	conversations                         map[string]*conversationLock
}

func (router *Router) SetPersistenceEnabled(enabled bool) {
	if router == nil {
		return
	}
	router.persistenceEnabled.Store(enabled)
}

func (router *Router) persistenceWritesEnabled() bool {
	return router != nil && router.persistenceEnabled.Load()
}

func (router *Router) SetPreferredAccount(accountID string) {
	if router == nil {
		return
	}
	router.mu.Lock()
	router.preferred = strings.TrimSpace(accountID)
	router.preferredUsed = false
	router.mu.Unlock()
}

func (router *Router) ReleaseSessionAffinity(ctx context.Context, sessionID string) error {
	if router == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if err := router.affinity.forget(ctx, affinityKeys{session: sessionID}); err != nil {
		return err
	}
	return router.affinity.forget(ctx, affinityKeys{session: "__compact_session__:" + sessionID})
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
	if config.PrecommitWatchdogBudget <= 0 {
		// Reference 0.437.8: max(300_000-ms stream idle, 30_000-ms
		// precommit recovery epoch), see recovery_deadline.rs.
		config.PrecommitWatchdogBudget = 5 * time.Minute
	}
	if config.SelectionSequenceSeed == 0 {
		config.SelectionSequenceSeed = newSelectionSequenceSeed()
	}
	router := &Router{
		source: config.Accounts, gateway: config.Gateway,
		quota:     config.QuotaPreflight,
		state:     config.RoutingState,
		activity:  config.Activity,
		preferred: strings.TrimSpace(config.PreferredAccount),
		now:       config.Now, wait: config.Wait, maxInspect: config.MaxInspectBytes,
		affinity: newAffinityStore(), quarantine: make(map[string]quarantineState),
		ephemeralCredentialIDs: make(map[string]struct{}),
		inflight:               make(map[string]int), inflightChanged: make(chan struct{}),
		profileInflightHardLimit: config.ProfileInflightHardLimit, profileInflightWait: config.ProfileInflightWait,
		precommitWatchdogBudget: config.PrecommitWatchdogBudget,
		quotaBlocked:            make(map[string]bool), quotaChecks: make(map[quotaCheckKey]quotaCheck),
		routeHealth:              make(map[routeHealthKey]routingentity.RouteHealthScore),
		routeMemory:              make(map[routeMemoryKey]routingentity.RouteMemoryScore),
		routeCircuits:            make(map[routeHealthKey]routingentity.RouteCircuit),
		transportBackoffs:        make(map[routeHealthKey]routingentity.TransportBackoff),
		accountHomes:             make(map[string]string),
		previousResponseFailures: make(map[previousResponseFailureKey]routingentity.PreviousResponseFailure),
		promptCacheBindings:      make(map[string]promptCacheBinding),
		autoRedeem:               config.AutoRedeem, redeemer: config.Redeemer,
	}
	router.selection.sequence.Store(config.SelectionSequenceSeed)
	router.persistenceEnabled.Store(true)
	if err := router.initializeAffinityPersistence(config); err != nil {
		return nil, fmt.Errorf("load continuation statuses: %w", err)
	}
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
		memory, err := config.RoutingState.LoadRouteMemory(ctx, now)
		if err != nil {
			return nil, err
		}
		for _, score := range memory {
			router.routeMemory[routeMemoryKey{accountID: score.AccountID, route: score.Route, kind: score.Kind}] = score
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

func (router *Router) forwardWithoutWatchdog(ctx context.Context, request proxymodel.Request) (*Exchange, error) {
	request.SelectionSequence = router.selection.sequence.Add(1)
	request = router.scrubDeadTurnStateContext(ctx, request)
	promptCacheKey := requestPromptCacheKey(request)
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
	if err != nil {
		return nil, err
	}
	if owner != "" && keys.hasSoftSessionAffinity(request.QuotaSelection) &&
		router.softSessionOwnerBlocked(accounts, owner, request.QuotaSelection) && router.affinity.repository != nil {
		if durableRelease == nil {
			if durableRelease, err = router.affinity.repository.AcquireConversation(ctx); err != nil {
				return nil, err
			}
		}
		owner, err = router.affinity.refreshOwner(ctx, keys, router.now())
		if err != nil {
			_ = durableRelease()
			return nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
		}
	}
	if durableRelease != nil {
		defer durableRelease()
	}
	restorePreviousResponseTurnState(ctx, &request, accounts, owner, &keys, router.affinity, router.now())
	result, err := router.routeRequest(ctx, request, accounts, owner, &keys)
	if err != nil {
		return nil, err
	}
	if owner != "" && keys.hasSoftSessionAffinity(request.QuotaSelection) &&
		result.AccountID != owner && result.Response != nil && !result.Failed && result.Response.StatusCode < 400 {
		if err := router.affinity.forgetOwned(ctx, owner, affinityKeys{session: keys.session}, router.now()); err != nil {
			_ = result.Response.Body.Close()
			return nil, err
		}
	}
	// Carry the selected provider policy into both startup and late-stream
	// response observation. Account IDs alone do not encode provider type.
	for _, account := range accounts {
		if account.ID == result.AccountID {
			result.ProviderKind = account.Provider.Kind
			break
		}
	}
	if err := router.bindSuccessfulResponse(
		ctx, &result, accounts, keys, request.WebSocketMessage, request.QuotaSelection, promptCacheKey,
	); err != nil {
		return nil, err
	}
	transferred = true
	return &Exchange{Result: result, release: release}, nil
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
		return router.observedGatewayAttempt(ctx, request, account, func() (*proxymodel.Response, error) {
			return router.gateway.Execute(ctx, request, account)
		})
	}
	return router.observedGatewayAttempt(ctx, request, account, func() (*proxymodel.Response, error) {
		return router.gateway.Execute(ctx, request, account)
	})
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
