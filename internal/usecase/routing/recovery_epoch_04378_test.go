package routing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type epochTransportGateway04378 struct {
	attempts  int
	onFailure func()
}

func (gateway *epochTransportGateway04378) Execute(
	_ context.Context, _ proxymodel.Request, _ proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.attempts++
	if gateway.attempts == 1 {
		gateway.onFailure()
		return nil, io.ErrUnexpectedEOF
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader("{}")),
	}, nil
}

// Prodex 0.437.8 keeps a request-scoped recovery epoch pending when a profile
// is still cooling, even when a 30-second wait slice cannot cover that cooldown.
// Non-watchdog routes retain their finite transport recovery behavior.
func TestProdex04378PrecommitTransportCooldownSurvivesMultipleWaitEpochs(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		route      quotamodel.RouteKind
		recover    bool
	}{
		{"responses watchdog", "/backend-api/godex/responses", quotamodel.RouteKindResponses, true},
		{"compact watchdog", "/v1/responses/compact", quotamodel.RouteKindCompact, true},
		{"ordinary standard request", "/v1/chat/completions", quotamodel.RouteKindStandard, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(1_000_000, 0)
			var waits []time.Duration
			gateway := &epochTransportGateway04378{}
			var router *Router
			gateway.onFailure = func() {
				// A concurrent failure can extend the shared route cooldown
				// after this request selected its first upstream candidate.
				route := routeHealthRoute(tc.route)
				router.mu.Lock()
				router.transportBackoffs[routeHealthKey{accountID: "account-a", route: route}] =
					routingentity.TransportBackoff{AccountID: "account-a", Route: route, UntilUnix: now.Unix() + 90}
				router.mu.Unlock()
			}
			var err error
			router, err = NewRouter(Config{
				Gateway: gateway, Now: func() time.Time { return now },
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/a", Enabled: true}}, nil
				},
				Wait: func(ctx context.Context, delay time.Duration) error {
					if delay <= 0 || delay > maxFreshRecoveryWait || len(waits) >= 10 {
						t.Fatalf("invalid recovery wait %s after %v", delay, waits)
					}
					waits = append(waits, delay)
					now = now.Add(delay)
					return ctx.Err()
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(t.Context(), proxymodel.Request{
				Method: http.MethodPost, Path: tc.path, QuotaSelection: quotamodel.Selection{RouteKind: tc.route},
			})
			if tc.recover {
				if err != nil {
					t.Fatalf("request returned before eligible cooldown ended: %v, waits=%v, attempts=%d", err, waits, gateway.attempts)
				}
				defer exchange.Close()
				var waited time.Duration
				for _, delay := range waits {
					waited += delay
				}
				if gateway.attempts != 2 || len(waits) < 3 || waited < 90*time.Second ||
					exchange.Result.Response.StatusCode != http.StatusOK {
					t.Fatalf("recovered attempts=%d waits=%v status=%d", gateway.attempts, waits, exchange.Result.Response.StatusCode)
				}
				return
			}
			if exchange != nil {
				_ = exchange.Close()
				t.Fatal("ordinary route retried transport after a capped cooldown epoch")
			}
			var presented *proxymodel.Error
			if !errors.As(err, &presented) || presented.StatusCode != http.StatusServiceUnavailable ||
				gateway.attempts != 1 || len(waits) != 1 {
				t.Fatalf("ordinary route status=%v attempts=%d waits=%v", err, gateway.attempts, waits)
			}
		})
	}
}

func TestProdex04378CooldownWaitUsesOneAbsolutePrecommitWatchdog(t *testing.T) {
	gateway := &epochTransportGateway04378{onFailure: func() {}}
	waitEntered := false
	router, err := NewRouter(Config{
		Gateway: gateway, PrecommitWatchdogBudget: 40 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/a", Enabled: true}}, nil
		},
		Wait: func(ctx context.Context, delay time.Duration) error {
			waitEntered = true
			if delay <= 0 || delay > maxFreshRecoveryWait {
				t.Errorf("uncapped cooldown wait = %s", delay)
			}
			<-ctx.Done()
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/v1/responses",
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	})
	if exchange != nil {
		_ = exchange.Close()
		t.Fatal("expired watchdog returned a response")
	}
	var presented *proxymodel.Error
	if !errors.As(err, &presented) || presented.StatusCode != http.StatusGatewayTimeout ||
		!waitEntered || gateway.attempts != 1 {
		t.Fatalf("cooldown watchdog error=%v entered=%t attempts=%d", err, waitEntered, gateway.attempts)
	}
}
