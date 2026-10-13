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

const precommitWatchdogMessage04378 = "request timed out while waiting for upstream recovery before output started"

var errPrecommitWatchdog04378 = errors.New(precommitWatchdogMessage04378)

func requiresPrecommitWatchdog04378(request proxymodel.Request) bool {
	if request.WebSocketMessage {
		return true
	}
	path := strings.TrimRight(request.Path, "/")
	return strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/responses/compact")
}

func (router *Router) Forward(ctx context.Context, request proxymodel.Request) (*Exchange, error) {
	if !requiresPrecommitWatchdog04378(request) {
		return router.forwardWithoutWatchdog(ctx, request)
	}
	scopedCtx, cancel := context.WithCancelCause(ctx)
	var mu sync.Mutex
	completed := false
	timer := time.AfterFunc(router.precommitWatchdogBudget, func() {
		mu.Lock()
		if !completed {
			cancel(errPrecommitWatchdog04378)
		}
		mu.Unlock()
	})
	exchange, err := router.forwardWithoutWatchdog(scopedCtx, request)
	mu.Lock()
	completed = true
	timer.Stop()
	expired := errors.Is(context.Cause(scopedCtx), errPrecommitWatchdog04378)
	mu.Unlock()
	if expired {
		if exchange != nil {
			_ = exchange.Close()
		}
		cancel(nil)
		return nil, &proxymodel.Error{StatusCode: http.StatusGatewayTimeout, Message: precommitWatchdogMessage04378}
	}
	if err != nil {
		cancel(nil)
		return nil, err
	}
	if exchange == nil {
		cancel(nil)
		return nil, errors.New("precommit routing returned no exchange")
	}
	release := exchange.release
	exchange.release = func() {
		if release != nil {
			release()
		}
		cancel(nil)
	}
	return exchange, nil
}
