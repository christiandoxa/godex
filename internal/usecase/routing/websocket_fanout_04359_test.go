package routing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketCapacity04359Gateway struct {
	mu          sync.Mutex
	dispatches  map[uint64]int
	maxInflight int
	router      *Router
}

func (*websocketCapacity04359Gateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	return nil, fmt.Errorf("websocket message incorrectly dispatched as an HTTP request")
}

func (g *websocketCapacity04359Gateway) ExecuteWebSocketMessage(
	_ context.Context, req proxymodel.Request, _ proxymodel.Account,
) (*proxymodel.Response, error) {
	id := req.RequestID
	g.router.mu.Lock()
	observedInflight := g.router.inflight["main"]
	g.router.mu.Unlock()
	g.mu.Lock()
	g.dispatches[id]++
	if observedInflight > g.maxInflight {
		g.maxInflight = observedInflight
	}
	g.mu.Unlock()
	payload := fmt.Sprintf(`{"type":"response.output_text.delta","delta":"worker-%d"}
{"type":"response.completed","response":{"id":"resp-ws-04359-%d"}}`, id, id)
	body := io.NopCloser(strings.NewReader(payload))
	return &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                body,
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: fmt.Sprintf("resp-ws-04359-%d", id),
	}, nil
}

// Prodex 0.435.9 uses two simultaneous parent groups with sixteen workers.
// This exercises the Godex WebSocket-message routing path, not a synthetic
// HTTP-only route. Every request must retain its own payload, be dispatched
// exactly once, and respect the weighted profile hard limit while queued.
func TestProdex04359WebSocketTwoParentFanoutIsolatedAndCapacityBounded(t *testing.T) {
	const parents = 2
	const workers = 16
	const total = parents * (workers + 1)
	gateway := &websocketCapacity04359Gateway{dispatches: make(map[uint64]int)}
	var waits atomic.Int32
	router, err := NewRouter(Config{
		Gateway:                  gateway,
		ProfileInflightHardLimit: 2,
		ProfileInflightWait: func(ctx context.Context, changed <-chan struct{}, epoch time.Duration) error {
			waits.Add(1)
			return waitProfileInflightSignalOrEpoch(ctx, changed, epoch)
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Home: "/main", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway.router = router
	defer router.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	start := make(chan struct{})
	errs := make(chan error, total)
	var group sync.WaitGroup
	for parent := 0; parent < parents; parent++ {
		for worker := 0; worker <= workers; worker++ {
			id := uint64(parent*(workers+1) + worker + 1)
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				req := websocketDispatchRequest(fmt.Sprintf(`{"type":"response.create","session_id":"session-%d","input":[{"type":"message","role":"user","content":"work for %d"}]}`, id, id), id)
				req.RequestID = id
				outcome, err := router.Forward(ctx, req)
				if err != nil {
					errs <- fmt.Errorf("request %d: %w", id, err)
					return
				}
				// Keep the permit held before EOF; the router releases it on EOF,
				// which may precede the caller's later Close().
				time.Sleep(6 * time.Millisecond)
				payload, readErr := io.ReadAll(outcome.Result.Response.Body)
				if readErr != nil {
					errs <- fmt.Errorf("request %d read: %w", id, readErr)
				}
				if outcome.Result.AccountID != "main" ||
					strings.Count(string(payload), "response.output_text.delta") != 1 ||
					strings.Count(string(payload), "response.completed") != 1 ||
					!strings.Contains(string(payload), fmt.Sprintf("resp-ws-04359-%d", id)) ||
					!strings.Contains(string(payload), fmt.Sprintf("worker-%d", id)) {
					errs <- fmt.Errorf("request %d mixed response %q, owner %q", id, payload, outcome.Result.AccountID)
				}
				if err := outcome.Close(); err != nil {
					errs <- fmt.Errorf("request %d close: %w", id, err)
				}
			}()
		}
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	router.mu.Lock()
	remaining := router.inflight["main"]
	admissions := router.profileInflightAdmissionsTotal
	releases := router.profileInflightReleasesTotal
	router.mu.Unlock()
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if waits.Load() == 0 {
		t.Error("fanout did not exercise profile inflight wait")
	}
	if remaining != 0 || gateway.maxInflight != 2 || admissions != total || releases != total {
		t.Errorf("inflight/max-weight/admissions/releases = %d/%d/%d/%d, want 0/2/%d/%d",
			remaining, gateway.maxInflight, admissions, releases, total, total)
	}
	if len(gateway.dispatches) != total {
		t.Fatalf("unique upstream messages = %d, want %d", len(gateway.dispatches), total)
	}
	for id := uint64(1); id <= total; id++ {
		if count := gateway.dispatches[id]; count != 1 {
			t.Errorf("websocket session %d dispatched %d times", id, count)
		}
	}
}
