package routing

import (
	"context"
	"errors"
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

// 0.435.9: the capacity predicate and release notification form one
// atomic handoff; a release before waiter registration is not lost.
func TestProdex04359ReleaseBeforeWaitDoesNotConsumeLocalEpoch(t *testing.T) {
	var calls atomic.Int32
	router := &Router{
		inflight:                 map[string]int{"main": 2},
		inflightChanged:          make(chan struct{}),
		profileInflightHardLimit: 2,
		profileInflightWait: func(context.Context, <-chan struct{}, time.Duration) error {
			calls.Add(1)
			return errors.New("missed release notification")
		},
	}
	req := profileInflightRequest("/backend-api/codex/responses")
	if !router.profileInflightHardLimitedForRequest("main", req) {
		t.Fatal("setup must begin saturated")
	}
	// Simulate the permit release after selection sees a saturated profile
	// but before the waiter begins observing the release generation.
	router.mu.Lock()
	delete(router.inflight, "main")
	close(router.inflightChanged)
	router.inflightChanged = make(chan struct{})
	router.mu.Unlock()

	if err := router.waitForProfileInflight(
		context.Background(), req, []proxymodel.Account{{ID: "main"}},
	); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("released permit still waited for an epoch: %d", calls.Load())
	}
}

// Exercise the actual router with two concurrent parent groups of sixteen
// requests, ensuring admission capacity delays requests rather than issuing
// multiple upstream attempts or mixing individual request IDs.
type fanout04359Gateway struct {
	mu   sync.Mutex
	seen map[uint64]int
}

func (g *fanout04359Gateway) Execute(_ context.Context, req proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	g.mu.Lock()
	g.seen[req.RequestID]++
	g.mu.Unlock()
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"worker-%d\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-04359-%d\"}}\n\n",
			req.RequestID, req.RequestID,
		))),
	}, nil
}

func TestProdex04359TwoParentFanoutWaitsWithoutUpstreamRetries(t *testing.T) {
	const parents, workersPerParent = 2, 16
	gateway := &fanout04359Gateway{seen: make(map[uint64]int)}
	router, err := NewRouter(Config{
		Gateway:                  gateway,
		ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Home: "/main", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	start := make(chan struct{})
	problems := make(chan error, parents*workersPerParent)
	var group sync.WaitGroup
	for parent := 0; parent < parents; parent++ {
		for worker := 0; worker < workersPerParent; worker++ {
			id := uint64(parent*workersPerParent + worker + 1)
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				req := profileInflightRequest("/backend-api/codex/responses")
				req.RequestID = id
				result, err := router.Forward(ctx, req)
				if err != nil {
					problems <- fmt.Errorf("request %d: %w", id, err)
					return
				}
				if result.Result.AccountID != "main" {
					problems <- fmt.Errorf("request %d: account %q", id, result.Result.AccountID)
				}
				remaining, readErr := io.ReadAll(result.Result.Response.Body)
				if readErr != nil {
					problems <- fmt.Errorf("request %d: read stream: %w", id, readErr)
				}
				body := string(result.Result.Prefix) + string(remaining)
				if strings.Count(body, "response.output_text.delta") != 1 ||
					strings.Count(body, "response.completed") != 1 ||
					!strings.Contains(body, fmt.Sprintf("resp-04359-%d", id)) ||
					!strings.Contains(body, fmt.Sprintf("worker-%d", id)) {
					problems <- fmt.Errorf("request %d: mixed or duplicated SSE events: %q", id, body)
				}
				time.Sleep(8 * time.Millisecond)
				if err := result.Close(); err != nil {
					problems <- fmt.Errorf("request %d: close: %w", id, err)
				}
			}()
		}
	}
	close(start)
	group.Wait()
	close(problems)
	for err := range problems {
		t.Error(err)
	}
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.seen) != parents*workersPerParent {
		t.Fatalf("upstream request IDs = %d, want %d", len(gateway.seen), parents*workersPerParent)
	}
	for id := uint64(1); id <= parents*workersPerParent; id++ {
		if gateway.seen[id] != 1 {
			t.Errorf("request %d dispatched %d times", id, gateway.seen[id])
		}
	}
}

type capacityRetry04359Gateway struct {
	attempts atomic.Int32
}

func (g *capacityRetry04359Gateway) Execute(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
	g.attempts.Add(1)
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("upstream rate limit")), PrecommitFailure: &proxymodel.PrecommitFailure{Code: "rate_limit_exceeded"}}, nil
}

// Synthetic capacity epochs must not advance the exponential backoff index
// of a genuine upstream rate-limit response after a permit is acquired.
func TestProdex04359UnsentCapacitySweepsDoNotIncreaseUpstreamRecoveryDelay(t *testing.T) {
	const capacityEpochs = 4
	sentinel := errors.New("observed upstream retry clock")
	var waitCalls atomic.Int32
	var clockCalls atomic.Int64
	var release func()
	var retryDelay time.Duration
	gateway := &capacityRetry04359Gateway{}
	router, err := NewRouter(Config{
		Gateway: gateway, ProfileInflightHardLimit: 1,
		Now: func() time.Time { return time.Unix(100+clockCalls.Add(1)*90, 0) },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Home: "/main", Enabled: true}}, nil
		},
		ProfileInflightWait: func(ctx context.Context, _ <-chan struct{}, _ time.Duration) error {
			if waitCalls.Add(1) == capacityEpochs {
				release()
			}
			return ctx.Err()
		},
		Wait: func(_ context.Context, d time.Duration) error {
			retryDelay = d
			return sentinel
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := profileInflightRequest("/v1/chat/completions")
	release, ok := router.tryAcquireProfileInflight("main", req, false)
	if !ok {
		t.Fatal("cannot reserve synthetic permit")
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_, err = router.Forward(ctx, req)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected upstream recovery sentinel, got %v", err)
	}
	if got := waitCalls.Load(); got != capacityEpochs {
		t.Fatalf("capacity epochs %d, want %d", got, capacityEpochs)
	}
	if got := gateway.attempts.Load(); got != 1 {
		t.Fatalf("upstream attempts=%d, want 1", got)
	}
	if retryDelay <= 0 || retryDelay >= time.Second {
		t.Fatalf("upstream retry backoff %s reflects %d unsent capacity epochs", retryDelay, capacityEpochs)
	}
}
