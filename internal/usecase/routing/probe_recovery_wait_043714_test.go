package routing

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type waitingQuotaProof043714 struct {
	*causalQuotaReceiptFixture043714
	entered  chan struct{}
	complete chan struct{}
}

func (q *waitingQuotaProof043714) ProbeRefreshRevision() uint64 { return 0 }
func (q *waitingQuotaProof043714) WaitProbeRefresh(ctx context.Context, _ uint64) bool {
	close(q.entered)
	select {
	case <-q.complete:
		return true
	case <-ctx.Done():
		return false
	}
}

// A real caller sees one recovered upstream attempt after the queued quota
// worker reports progress; it must never send before the authenticated proof.
func TestProdex043714EmptyAuthPoolWaitsForConfirmedProbeWithoutDoubleDispatch(t *testing.T) {
	for _, approved := range []bool{true, false} {
		name := "approved"
		if !approved {
			name = "unknown cached only"
		}
		t.Run(name, func(t *testing.T) {
			base := time.Unix(7100, 0)
			clock := base
			digest := [32]byte{4, 3, 2}
			selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
			account := proxymodel.Account{ID: "profile-a", Home: "/synthetic", Enabled: true}
			baseProof := &causalQuotaReceiptFixture043714{
				valid: false, availability: quotamodel.Availability{Ready: true, Source: quotamodel.SourceLive},
				proof: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: digest, StartedAt: base.Add(3 * time.Second)},
			}
			quota := &waitingQuotaProof043714{causalQuotaReceiptFixture043714: baseProof, entered: make(chan struct{}), complete: make(chan struct{})}
			gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
				account.ID: {{status: http.StatusOK, body: `{"id":"recovered"}`}},
			}}
			router, err := NewRouter(Config{Gateway: gateway, QuotaPreflight: quota,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return []proxymodel.Account{account}, nil },
				Now:      func() time.Time { return clock },
			})
			if err != nil {
				t.Fatal(err)
			}
			response := &proxymodel.Response{StatusCode: 401, AuthCredentialKnown: true,
				AuthCredentialDigest: digest, AuthRequestStartedAt: base.Add(-time.Second)}
			router.recordRouteOutcome(t.Context(), account.ID, selection, response, responseOutcome{kind: responseAuthFailure})
			router.applyRetryOutcomeForAccount(t.Context(), account, selection, responseOutcome{kind: responseAuthFailure})
			clock = base.Add(5 * time.Second)
			go func() {
				<-quota.entered
				baseProof.valid = approved
				close(quota.complete)
			}()
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			forwarded, err := router.Forward(ctx, proxymodel.Request{QuotaSelection: selection})
			if approved {
				if err != nil || forwarded.Result.AccountID != account.ID ||
					forwarded.Result.Response.StatusCode != http.StatusOK ||
					strings.Join(gateway.calls, ",") != account.ID {
					t.Fatalf("recovered request not dispatched exactly once: result=%+v err=%v calls=%v", forwarded.Result, err, gateway.calls)
				}
				if err := forwarded.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || len(gateway.calls) != 0 {
					t.Fatalf("progress without trusted receipt dispatched request: err=%v calls=%v", err, gateway.calls)
				}
			}
		})
	}
}
