package proxy

import (
	"context"
	"errors"
	"time"
)

const requestRecoveryTimeoutMessage = "request timed out while waiting for upstream recovery before output started"

type requestDeadlineContextKey struct{}

type requestDeadlineScope struct {
	parent   context.Context
	deadline time.Time
	local    bool
}

func withRequestDeadline(parent context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if _, scoped := requestDeadlineScopeFor(parent); scoped {
		return parent, func() {}
	}
	if budget <= 0 {
		return parent, func() {}
	}
	deadline := time.Now().Add(budget)
	parentDeadline, hasParentDeadline := parent.Deadline()
	scope := requestDeadlineScope{
		parent: parent, deadline: deadline,
		local: !hasParentDeadline || deadline.Before(parentDeadline),
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	return context.WithValue(ctx, requestDeadlineContextKey{}, scope), cancel
}

func requestDeadlineScopeFor(ctx context.Context) (requestDeadlineScope, bool) {
	if ctx == nil {
		return requestDeadlineScope{}, false
	}
	scope, ok := ctx.Value(requestDeadlineContextKey{}).(requestDeadlineScope)
	return scope, ok && scope.parent != nil
}

func requestDeadlineTimedOut(ctx context.Context) bool {
	scope, ok := requestDeadlineScopeFor(ctx)
	return ok && scope.local && errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func withoutRequestDeadline(ctx context.Context) context.Context {
	if scope, ok := requestDeadlineScopeFor(ctx); ok {
		return scope.parent
	}
	return ctx
}

// detachRequestDeadline keeps the local watchdog while dropping a client
// disconnect cancellation so captured requests can finish their bounded work.
func detachRequestDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	scope, ok := requestDeadlineScopeFor(ctx)
	if !ok {
		return context.WithoutCancel(ctx), func() {}
	}
	base := context.WithoutCancel(scope.parent)
	if !scope.local {
		return base, func() {}
	}
	remaining, cancel := context.WithDeadline(base, scope.deadline)
	return context.WithValue(remaining, requestDeadlineContextKey{}, scope), cancel
}
