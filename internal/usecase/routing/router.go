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
type Config struct {
	Accounts         func(context.Context) ([]proxymodel.Account, error)
	PreferredAccount string
	Now              func() time.Time
	MaxInspectBytes  int64
	Gateway          gateway
	Bindings         bindingRepository
}

type Router struct {
	source        func(context.Context) ([]proxymodel.Account, error)
	gateway       gateway
	preferred     string
	now           func() time.Time
	maxInspect    int64
	affinity      *affinityStore
	mu            sync.Mutex
	cursor        int
	preferredUsed bool
	quarantine    map[string]time.Time
	conversations map[string]*conversationLock
}

func NewRouter(config Config) (*Router, error) {
	if config.Accounts == nil {
		return nil, errors.New("routing account source is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.MaxInspectBytes <= 0 {
		config.MaxInspectBytes = 64 << 10
	}
	router := &Router{source: config.Accounts, gateway: config.Gateway, preferred: strings.TrimSpace(config.PreferredAccount), now: config.Now, maxInspect: config.MaxInspectBytes, affinity: newAffinityStore(), quarantine: make(map[string]time.Time)}
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
	keys := requestAffinity(request, request.Body)
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
	accounts, err := router.source(ctx)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: 503, Message: "cannot load managed accounts"}
	}
	accounts = sortRuntimeAccounts(accounts)
	owner, err := router.affinity.owner(ctx, keys, router.now())
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
	}
	if owner == "" && router.affinity.repository != nil && (keys.thread != "" || keys.session != "") {
		unlock, err := router.affinity.repository.AcquireConversation(ctx)
		if err != nil {
			return nil, err
		}
		defer unlock()
		// Another process may have committed ownership while this request waited.
		owner, err = router.affinity.owner(ctx, keys, router.now())
		if err != nil {
			return nil, err
		}
	}
	if owner == "" && (keys.previous != "" || keys.turn != "") {
		return nil, &proxymodel.Error{StatusCode: 409, Message: "continuation owner is unknown; continuity was preserved"}
	}
	var result proxymodel.Forwarded
	if owner != "" {
		result, err = router.forwardBound(ctx, request, accounts, owner)
	} else {
		result, err = router.forwardFresh(ctx, request, accounts)
	}
	if err != nil {
		return nil, err
	}
	if result.Response.StatusCode < 400 {
		stream := strings.Contains(strings.ToLower(result.Response.Header.Get("Content-Type")), "text/event-stream")
		if result.Prefix == nil && !stream {
			result.Prefix, _ = inspectResponse(result.Response.Body, router.maxInspect)
		}
		if err := router.affinity.remember(ctx, result.AccountID, keys, router.now()); err != nil {
			result.Response.Body.Close()
			return nil, err
		}
		if err := router.Observe(ctx, result.AccountID, result.Response.Header, result.Prefix, stream); err != nil {
			result.Response.Body.Close()
			return nil, err
		}
	}
	transferred = true
	return &Exchange{Result: result, release: release}, nil
}

func (router *Router) Observe(ctx context.Context, accountID string, headers http.Header, body []byte, stream bool) error {
	return router.affinity.remember(ctx, accountID, responseAffinity(headers, body, stream), router.now())
}

func (router *Router) execute(ctx context.Context, request proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	if router.gateway == nil {
		return nil, errors.New("routing gateway is not configured")
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
