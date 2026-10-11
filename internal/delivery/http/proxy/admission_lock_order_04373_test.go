package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdmissionOwnerProjectionRefreshesAfterSelectionNotification(t *testing.T) {
	handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), admissionLimits{
		global: 2,
		lane: [admissionLaneCount]int{
			admissionLaneResponses: 1,
			admissionLaneCompact:   1,
			admissionLaneWebSocket: 1,
			admissionLaneStandard:  1,
		},
	}).(*activeRequestHandler)

	var runtimeMu sync.Mutex
	var projections atomic.Int32
	var owner atomic.Bool
	firstProjected := make(chan struct{})
	allowFirst := make(chan struct{})
	secondProjected := make(chan struct{})
	handler.ownedAdmission = func(context.Context, admissionLane, http.Header, []byte) bool {
		switch projections.Add(1) {
		case 1:
			close(firstProjected)
			<-allowFirst
			return false
		case 2:
			close(secondProjected)
			runtimeMu.Lock()
			defer runtimeMu.Unlock()
			return owner.Load()
		default:
			return owner.Load()
		}
	}

	waiterRegistered := make(chan struct{})
	handler.wait = func(ctx context.Context, changed <-chan struct{}) bool {
		close(waiterRegistered)
		return waitAdmissionSignal(ctx, changed)
	}

	if !handler.acquire(context.Background(), admissionLaneResponses) {
		t.Fatal("failed to hold the saturated response lane")
	}
	holdReleased := false
	defer func() {
		if !holdReleased {
			handler.release(admissionLaneResponses)
		}
	}()

	request := httptest.NewRequest(http.MethodPost, "/backend-api/codex/responses", nil)
	result := make(chan bool, 1)
	go func() { result <- handler.acquireRequest(request, admissionLaneResponses) }()
	select {
	case <-firstProjected:
	case <-time.After(time.Second):
		t.Fatal("admission waiter did not project initial ownership")
	}
	close(allowFirst)
	select {
	case <-waiterRegistered:
	case <-time.After(time.Second):
		t.Fatal("admission waiter did not register before notification")
	}

	runtimeMu.Lock()
	owner.Store(true)
	handler.notifySelectionChange()
	select {
	case <-secondProjected:
	case <-time.After(time.Second):
		runtimeMu.Unlock()
		t.Fatal("admission waiter did not re-project ownership after notification")
	}
	mutexAcquired := make(chan struct{})
	go func() {
		handler.mu.Lock()
		close(mutexAcquired)
		handler.mu.Unlock()
	}()
	select {
	case <-mutexAcquired:
	case <-time.After(time.Second):
		runtimeMu.Unlock()
		t.Fatal("admission notification mutex was held while projecting runtime ownership")
	}
	runtimeMu.Unlock()

	select {
	case acquired := <-result:
		if !acquired {
			t.Fatal("owned admission did not bypass the saturated lane")
		}
		handler.release(admissionLaneResponses)
		holdReleased = true
	case <-time.After(time.Second):
		t.Fatal("owned admission waiter did not finish after re-projection")
	}
}
