package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const requestRecoveryTimeoutMessage = "request timed out while waiting for upstream recovery before output started"

var errLocalRequestDeadline = fmt.Errorf("local request deadline: %w", context.DeadlineExceeded)

type requestDeadlineContextKey struct{}
type requestAcceptedAtContextKey struct{}

type requestAcceptedAtProvider interface {
	AcceptedAt() time.Time
}

type requestDeadlineScope struct {
	parent   context.Context
	deadline time.Time
	local    bool
	watchdog *requestWatchdogContext
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
	return newRequestDeadlineContext(parent, scope)
}

// The local deadline can end at commitment without canceling the upstream body.
// Parent cancellation and parent deadlines remain owned by the caller.
type requestWatchdogContext struct {
	context.Context
	deadline  time.Time
	mu        sync.Mutex
	timer     *time.Timer
	committed bool
}

func newRequestDeadlineContext(parent context.Context, scope requestDeadlineScope) (context.Context, context.CancelFunc) {
	base, cancel := context.WithCancelCause(parent)
	watchdog := &requestWatchdogContext{Context: base, deadline: scope.deadline}
	remaining := time.Until(scope.deadline)
	if remaining <= 0 {
		cancel(errLocalRequestDeadline)
	} else {
		watchdog.timer = time.AfterFunc(remaining, func() {
			watchdog.mu.Lock()
			defer watchdog.mu.Unlock()
			if !watchdog.committed {
				cancel(errLocalRequestDeadline)
			}
		})
	}
	scope.watchdog = watchdog
	return context.WithValue(watchdog, requestDeadlineContextKey{}, scope), func() {
		watchdog.disarm()
		cancel(context.Canceled)
	}
}

func (ctx *requestWatchdogContext) Deadline() (time.Time, bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	parentDeadline, present := ctx.Context.Deadline()
	if ctx.committed || present && parentDeadline.Before(ctx.deadline) {
		return parentDeadline, present
	}
	return ctx.deadline, true
}

func (ctx *requestWatchdogContext) Err() error {
	err := ctx.Context.Err()
	if err != nil && errors.Is(context.Cause(ctx.Context), errLocalRequestDeadline) {
		return context.DeadlineExceeded
	}
	return err
}

func (ctx *requestWatchdogContext) disarm() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.committed = true
	if ctx.timer != nil {
		ctx.timer.Stop()
	}
}

func disarmRequestDeadline(ctx context.Context) {
	if scope, ok := requestDeadlineScopeFor(ctx); ok && scope.watchdog != nil {
		scope.watchdog.disarm()
	}
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
	return newRequestDeadlineContext(base, scope)
}
