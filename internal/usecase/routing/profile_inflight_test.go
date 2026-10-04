package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type profileInflightGateway struct {
	mu     sync.Mutex
	owners []string
}

func (gateway *profileInflightGateway) Execute(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.owners = append(gateway.owners, account.ID)
	gateway.mu.Unlock()
	headers := make(http.Header)
	headers.Set("Content-Type", "text/event-stream")
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n")),
	}, nil
}

func (gateway *profileInflightGateway) ownerTrace() string {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	return strings.Join(gateway.owners, ",")
}

func profileInflightRequest(path string) proxymodel.Request {
	return proxymodel.Request{Path: path, Header: make(http.Header)}
}

func TestProfileInflightPolicyMatchesTaggedWeightsAndHardLimit(t *testing.T) {
	websocket := profileInflightRequest("/backend-api/codex/responses")
	websocket.Header.Set("Upgrade", "websocket")
	if requestProfileInflightWeight(profileInflightRequest("/backend-api/codex/responses")) != 2 ||
		requestProfileInflightWeight(websocket) != 2 ||
		requestProfileInflightWeight(profileInflightRequest("/backend-api/codex/responses/compact")) != 1 ||
		requestProfileInflightWeight(profileInflightRequest("/v1/chat/completions")) != 1 {
		t.Fatal("profile in-flight weights do not match Prodex")
	}
	if effectiveProfileInflightHardLimit(0, 2) != 2 ||
		effectiveProfileInflightHardLimit(0, 1) != 1 ||
		effectiveProfileInflightHardLimit(8, 2) != 8 {
		t.Fatal("effective profile in-flight hard limit does not match Prodex")
	}
}

func TestProfileInflightSaturationWaitsForReleaseBeforeUpstreamDispatch(t *testing.T) {
	gateway := &profileInflightGateway{}
	waiting := make(chan struct{}, 1)
	router, err := NewRouter(Config{
		Gateway:                  gateway,
		ProfileInflightHardLimit: 2,
		ProfileInflightWait: func(ctx context.Context, changed <-chan struct{}, _ time.Duration) error {
			select {
			case waiting <- struct{}{}:
			default:
			}
			select {
			case <-changed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := profileInflightRequest("/backend-api/codex/responses")
	first, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	secondDone := make(chan error, 1)
	go func() {
		second, forwardErr := router.Forward(context.Background(), request)
		if forwardErr == nil {
			forwardErr = second.Close()
		}
		secondDone <- forwardErr
	}()
	select {
	case <-waiting:
	case err := <-secondDone:
		_ = first.Close()
		t.Fatalf("saturated request returned before waiting: %v", err)
	case <-time.After(5 * time.Second):
		_ = first.Close()
		t.Fatal("second request never entered profile in-flight wait")
	}
	if got := gateway.ownerTrace(); got != "a" {
		_ = first.Close()
		t.Fatalf("saturated request dispatched upstream: %q", got)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("profile capacity release did not resume request")
	}
	if got := gateway.ownerTrace(); got != "a,a" {
		t.Fatalf("owner trace after release = %q", got)
	}
}

func TestProfileInflightEpochReevaluatesEligibilityAndUsesAnotherProfile(t *testing.T) {
	gateway := &profileInflightGateway{}
	var mu sync.Mutex
	aEnabled, bEnabled := true, false
	epoch := make(chan struct{}, 1)
	router, err := NewRouter(Config{
		Gateway:                  gateway,
		ProfileInflightHardLimit: 2,
		ProfileInflightWait: func(context.Context, <-chan struct{}, time.Duration) error {
			mu.Lock()
			aEnabled, bEnabled = false, true
			mu.Unlock()
			epoch <- struct{}{}
			return nil
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			mu.Lock()
			defer mu.Unlock()
			return []proxymodel.Account{
				{ID: "a", Home: "/a", Enabled: aEnabled},
				{ID: "b", Home: "/b", Enabled: bEnabled},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := profileInflightRequest("/backend-api/codex/responses")
	first, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	second, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	select {
	case <-epoch:
	default:
		t.Fatal("saturated profile did not trigger an eligibility re-evaluation epoch")
	}
	if got := gateway.ownerTrace(); got != "a,b" {
		t.Fatalf("eligibility re-evaluation owner trace = %q, want a,b", got)
	}
}

func TestProfileInflightHardAffinityBypassesHardLimitButRemainsCounted(t *testing.T) {
	router, err := NewRouter(Config{
		Gateway:                  &profileInflightGateway{},
		ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := profileInflightRequest("/backend-api/codex/responses")
	release, ok := router.tryAcquireProfileInflight("a", request, false)
	if !ok {
		t.Fatal("initial profile permit rejected")
	}
	defer release()
	bypass, ok := router.tryAcquireProfileInflight("a", request, true)
	if !ok {
		t.Fatal("hard affinity did not bypass profile hard limit")
	}
	router.mu.Lock()
	count := router.inflight["a"]
	router.mu.Unlock()
	if count != 4 {
		bypass()
		t.Fatalf("hard affinity inflight count = %d, want 4", count)
	}
	bypass()
}

func TestProfileInflightWaitHonorsCancellation(t *testing.T) {
	waiting := make(chan struct{}, 1)
	gateway := &profileInflightGateway{}
	router, err := NewRouter(Config{
		Gateway:                  gateway,
		ProfileInflightHardLimit: 2,
		ProfileInflightWait: func(ctx context.Context, _ <-chan struct{}, _ time.Duration) error {
			waiting <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := profileInflightRequest("/backend-api/codex/responses")
	first, err := router.Forward(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, forwardErr := router.Forward(ctx, request)
		done <- forwardErr
	}()
	select {
	case <-waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("request never waited for profile capacity")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled profile capacity waiter unexpectedly succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled profile capacity waiter did not stop")
	}
	if got := gateway.ownerTrace(); got != "a" {
		t.Fatalf("canceled waiter dispatched upstream: %q", got)
	}
}
