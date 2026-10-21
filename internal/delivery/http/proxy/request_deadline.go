package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const requestRecoveryTimeoutMessage = "request timed out while waiting for upstream recovery before output started"

var errLocalRequestDeadline = fmt.Errorf("local request deadline: %w", context.DeadlineExceeded)

type requestDeadlineContextKey struct{}
type requestAcceptedAtContextKey struct{}

type connectionRequestStart struct {
	acceptedAt time.Time
	claimed    atomic.Bool
}

type requestAcceptedAtProvider interface {
	AcceptedAt() time.Time
}

type requestDeadlineScope struct {
	parent   context.Context
	deadline time.Time
	local    bool
	watchdog *requestWatchdog
}

func withRequestDeadline(parent context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if _, scoped := requestDeadlineScopeFor(parent); scoped {
		return parent, func() {}
	}
	if budget <= 0 {
		return parent, func() {}
	}
	start := time.Now()
	if acceptedAt, ok := claimRequestAcceptedAt(parent); ok && acceptedAt.Before(start) {
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

// Keep a stoppable application timer outside the context's stable parent deadline.
type requestWatchdog struct {
	mu        sync.Mutex
	timer     *time.Timer
	committed bool
}

func newRequestDeadlineContext(parent context.Context, scope requestDeadlineScope) (context.Context, context.CancelFunc) {
	base, cancel := context.WithCancelCause(parent)
	watchdog := &requestWatchdog{}
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
	return context.WithValue(base, requestDeadlineContextKey{}, scope), func() {
		watchdog.disarm()
		cancel(context.Canceled)
	}
}

func (watchdog *requestWatchdog) disarm() {
	watchdog.mu.Lock()
	defer watchdog.mu.Unlock()
	watchdog.committed = true
	if watchdog.timer != nil {
		watchdog.timer.Stop()
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
	start := &connectionRequestStart{acceptedAt: accepted.AcceptedAt()}
	return context.WithValue(ctx, requestAcceptedAtContextKey{}, start)
}

func claimRequestAcceptedAt(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	start, ok := ctx.Value(requestAcceptedAtContextKey{}).(*connectionRequestStart)
	// Queue age belongs to the first request, not every HTTP keep-alive turn.
	if !ok || start == nil || start.claimed.Swap(true) {
		return time.Time{}, false
	}
	return start.acceptedAt, !start.acceptedAt.IsZero()
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
	return ok && scope.local && (errors.Is(contextErr, context.DeadlineExceeded) ||
		contextErr != nil && errors.Is(context.Cause(ctx), errLocalRequestDeadline))
}

func withoutRequestDeadline(ctx context.Context) context.Context {
	if scope, ok := requestDeadlineScopeFor(ctx); ok {
		return scope.parent
	}
	return ctx
}

// detachRequestDeadline preserves the budget and any earlier parent deadline
// while dropping disconnect cancellation for closing connections.
func detachRequestDeadline(ctx context.Context) (context.Context, context.CancelFunc) {
	scope, ok := requestDeadlineScopeFor(ctx)
	if !ok {
		return context.WithoutCancel(ctx), func() {}
	}
	base := context.WithoutCancel(scope.parent)
	if deadline, present := scope.parent.Deadline(); present && deadline.Before(scope.deadline) {
		scope.deadline = deadline
	}
	return newRequestDeadlineContext(base, scope)
}
