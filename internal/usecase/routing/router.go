package routing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type gateway interface {
	Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error)
}
type AutoRedeemer interface {
	Try(context.Context, []proxymodel.Account, string, proxymodel.Request) (string, bool, error)
}

type Config struct {
	Accounts                 func(context.Context) ([]proxymodel.Account, error)
	PreferredAccount         string
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
	source                   func(context.Context) ([]proxymodel.Account, error)
	gateway                  gateway
	preferred                string
	now                      func() time.Time
	wait                     func(context.Context, time.Duration) error
	maxInspect               int64
	affinity                 *affinityStore
	mu                       sync.Mutex
	cursor                   int
	preferredUsed            bool
	inflight                 map[string]int
	inflightChanged          chan struct{}
	profileInflightHardLimit int
	profileInflightWait      profileInflightWaitFunc
	quarantine               map[string]quarantineState
	quotaBlocked             map[string]bool
	autoRedeem               bool
	redeemer                 AutoRedeemer
	conversations            map[string]*conversationLock
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
		preferred: strings.TrimSpace(config.PreferredAccount),
		now:       config.Now, wait: config.Wait, maxInspect: config.MaxInspectBytes,
		affinity: newAffinityStore(), quarantine: make(map[string]quarantineState),
		inflight: make(map[string]int), inflightChanged: make(chan struct{}),
		profileInflightHardLimit: config.ProfileInflightHardLimit, profileInflightWait: config.ProfileInflightWait,
		quotaBlocked: make(map[string]bool), autoRedeem: config.AutoRedeem, redeemer: config.Redeemer,
	}
	router.affinity.repository = config.Bindings
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
	owner, durableRelease, err := router.resolveOwner(ctx, keys)
	if durableRelease != nil {
		defer durableRelease()
	}
	if err != nil {
		return nil, err
	}
	result, err := router.routeRequest(ctx, request, accounts, owner)
	if err != nil {
		return nil, err
	}
	if err := router.bindSuccessfulResponse(ctx, &result, keys); err != nil {
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

func (router *Router) resolveOwner(ctx context.Context, keys affinityKeys) (string, func() error, error) {
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
	if owner == "" && opaqueContinuation(keys) {
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

func (router *Router) routeRequest(ctx context.Context, request proxymodel.Request, accounts []proxymodel.Account, owner string) (proxymodel.Forwarded, error) {
	if owner != "" {
		return router.forwardBound(ctx, request, accounts, owner)
	}
	return router.forwardFresh(ctx, request, accounts)
}

func (router *Router) bindSuccessfulResponse(ctx context.Context, result *proxymodel.Forwarded, keys affinityKeys) error {
	if result.Failed || result.Response.StatusCode >= 400 {
		return nil
	}
	if result.Response.WebSocketFrames {
		if !result.Response.FirstEventCommitted {
			return nil
		}
		if err := router.affinity.remember(ctx, result.AccountID, keys, router.now()); err != nil {
			result.Response.Body.Close()
			return err
		}
		responseKeys := websocketCommittedAffinity(result.Response)
		if err := router.affinity.remember(ctx, result.AccountID, responseKeys, router.now()); err != nil {
			result.Response.Body.Close()
			return err
		}
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
	return router.affinity.remember(ctx, accountID, responseAffinity(headers, body, stream), router.now())
}

func (router *Router) execute(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	if router.gateway == nil {
		return nil, errors.New("routing gateway is not configured")
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
