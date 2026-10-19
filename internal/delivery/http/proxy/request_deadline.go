package proxy

import (
	"context"
	"errors"
	"net"
	"time"
)

const requestRecoveryTimeoutMessage = "request timed out while waiting for upstream recovery before output started"

type requestDeadlineContextKey struct{}
type requestAcceptedAtContextKey struct{}

type requestAcceptedAtProvider interface {
	AcceptedAt() time.Time
}

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
	start := time.Now()
	if acceptedAt, ok := requestAcceptedAt(parent); ok && acceptedAt.Before(start) {
		start = acceptedAt
	}
	deadline := start.Add(budget)
	parentDeadline, hasParentDeadline := parent.Deadline()
	scope := requestDeadlineScope{
		parent: parent, deadline: deadline,
		local: !hasParentDeadline || deadline.Before(parentDeadline),
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	return context.WithValue(ctx, requestDeadlineContextKey{}, scope), cancel
}

func proxyConnContext(ctx context.Context, connection net.Conn) context.Context {
	accepted, ok := connection.(requestAcceptedAtProvider)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, requestAcceptedAtContextKey{}, accepted.AcceptedAt())
}

func requestAcceptedAt(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	acceptedAt, ok := ctx.Value(requestAcceptedAtContextKey{}).(time.Time)
	return acceptedAt, ok && !acceptedAt.IsZero()
}

func requestDeadlineScopeFor(ctx context.Context) (requestDeadlineScope, bool) {
	if ctx == nil {
		return requestDeadlineScope{}, false
	}
	scope, ok := ctx.Value(requestDeadlineContextKey{}).(requestDeadlineScope)
	return scope, ok && scope.parent != nil
}

// Classify the caller's error snapshot; a second read can race expiration.
func requestDeadlineTimedOut(ctx context.Context, contextErr error) bool {
	scope, ok := requestDeadlineScopeFor(ctx)
	return ok && scope.local && errors.Is(contextErr, context.DeadlineExceeded)
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
