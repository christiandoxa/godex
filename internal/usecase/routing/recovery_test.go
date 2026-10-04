package routing

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestFreshRequestWaitsForQuarantinedUsableAccount(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &sequenceRoutingGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		PreferredAccount: "a",
		Now:              func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("a", 15*time.Second)

	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "a" || exchange.Result.Response.StatusCode != http.StatusOK ||
		!reflect.DeepEqual(gateway.calls, []string{"a"}) || !reflect.DeepEqual(waits, []time.Duration{15 * time.Second}) {
		t.Fatalf("recovery owner/status/attempts/waits = %q/%d/%v/%v", exchange.Result.AccountID, exchange.Result.Response.StatusCode, gateway.calls, waits)
	}
}

func TestFreshTransientRecoveryContinuesPastSixteenSweepsWhileCandidateRemainsEligible(t *testing.T) {
	start := time.Unix(100, 0)
	now := start
	var waits []time.Duration
	fixtures := make([]routingResponseFixture, 18)
	for index := range fixtures[:17] {
		fixtures[index] = routingResponseFixture{status: http.StatusServiceUnavailable}
	}
	fixtures[17] = routingResponseFixture{status: http.StatusOK, body: `{"id":"recovered"}`}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{"a": fixtures}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Now: func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{RequestID: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	var waited time.Duration
	for _, delay := range waits {
		if delay <= 0 || delay > maxFreshRecoveryWait {
			t.Fatalf("recovery wait = %s, want a positive wait capped at %s", delay, maxFreshRecoveryWait)
		}
		waited += delay
	}
	if exchange.Result.Response.StatusCode != http.StatusOK || exchange.Result.Failed ||
		len(gateway.calls) != 18 || len(waits) != 17 ||
		waited <= 30*time.Second || !now.Equal(start.Add(waited)) {
		t.Fatalf("recovery status/failure/attempts/waits/time = %d/%t/%d/%d/%s", exchange.Result.Response.StatusCode, exchange.Result.Failed, len(gateway.calls), len(waits), waited)
	}
	for index, account := range gateway.calls {
		if account != "a" {
			t.Fatalf("attempt %d selected %q, want eligible account a", index+1, account)
		}
	}
}

func TestFreshTransientRecoveryContinuesPastSixtyFourAttemptsWhileCandidateRemainsEligible(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	const transientFailures = 65
	fixtures := make([]routingResponseFixture, transientFailures+1)
	for index := range fixtures[:transientFailures] {
		fixtures[index] = routingResponseFixture{status: http.StatusServiceUnavailable}
	}
	fixtures[transientFailures] = routingResponseFixture{status: http.StatusOK, body: "{\"id\":\"beyond-old-cap\"}"}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{"a": fixtures}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Now: func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{RequestID: 7})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.Response.StatusCode != http.StatusOK || exchange.Result.Failed ||
		len(gateway.calls) != transientFailures+1 || len(waits) != transientFailures {
		t.Fatalf("recovery status/failure/attempts/waits = %d/%t/%d/%d",
			exchange.Result.Response.StatusCode, exchange.Result.Failed, len(gateway.calls), len(waits))
	}
	for _, delay := range waits {
		if delay <= 0 || delay > maxFreshRecoveryWait {
			t.Fatalf("recovery wait = %s, want positive and <= %s", delay, maxFreshRecoveryWait)
		}
	}
}

func TestFreshRouteDoesNotWaitOnPreviouslyQuarantinedAuthFailure(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"a": {{status: http.StatusUnauthorized}, {status: http.StatusUnauthorized}},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Now: func() time.Time { return now },
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	if _, err := router.Forward(context.Background(), proxymodel.Request{}); err == nil {
		t.Fatal("second request unexpectedly recovered an auth-failed account")
	}
	if len(gateway.calls) != 2 || len(waits) != 0 {
		t.Fatalf("auth retry dispatches/waits = %d/%v, want 2/none", len(gateway.calls), waits)
	}
}
