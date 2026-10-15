package routing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

type transitionQuotaAccounts043714 struct{ homes map[string]string }

func (s transitionQuotaAccounts043714) List(context.Context) ([]accountentity.Account, error) {
	return nil, nil
}
func (s transitionQuotaAccounts043714) Current(context.Context) (accountentity.Account, error) {
	return accountentity.Account{ID: "main", Enabled: true}, nil
}
func (s transitionQuotaAccounts043714) Resolve(_ context.Context, id string) (accountentity.Account, error) {
	return accountentity.Account{ID: id, Enabled: true}, nil
}
func (s transitionQuotaAccounts043714) CodexHome(id string) string { return s.homes[id] }

type transitionUsage043714 struct {
	mu   sync.Mutex
	data map[string]quotamodel.Usage
}

func (s *transitionUsage043714) Fetch(_ context.Context, home string) (quotamodel.Usage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[home]
	if !ok {
		return quotamodel.Usage{}, fmt.Errorf("unknown synthetic profile %q", home)
	}
	return value, nil
}
func (s *transitionUsage043714) FetchAuthenticatedAtPolicy(ctx context.Context, home, _ string, _ bool) (quotamodel.Usage, quotamodel.AuthenticatedReceipt, error) {
	usage, err := s.Fetch(ctx, home)
	if err != nil {
		return quotamodel.Usage{}, quotamodel.AuthenticatedReceipt{}, err
	}
	// An isolated synthetic adapter stands in for the separately tested
	// gateway which binds its receipt to the exact HTTP bearer credential.
	return usage, quotamodel.AuthenticatedReceipt{
		Confirmed: true, CredentialDigest: [32]byte{7, 1, 4}, StartedAt: time.Now(),
	}, nil
}

func (s *transitionUsage043714) set(home string, usage quotamodel.Usage) {
	s.mu.Lock()
	s.data[home] = usage
	s.mu.Unlock()
}

func transitionUsageValue043714(five, weekly int64, prolite bool, now time.Time) quotamodel.Usage {
	fiveReset := now.Add(time.Hour).Unix()
	weeklyReset := now.Add(7 * 24 * time.Hour).Unix()
	usage := quotamodel.Usage{
		PlanType:  "plus",
		Primary:   &quotamodel.Window{UsedPercent: &five, ResetAt: &fiveReset},
		Secondary: &quotamodel.Window{UsedPercent: &weekly, ResetAt: &weeklyReset},
	}
	if prolite {
		usage.PlanType = "prolite"
		usage.Primary = nil
	}
	return usage
}

type transitionGateway043714 struct{ calls []string }

func (g *transitionGateway043714) Execute(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	g.calls = append(g.calls, account.ID)
	return &proxymodel.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"synthetic-ok"}`))}, nil
}

func TestProdex043714ThreeProfileQuotaResetAndProLiteWeeklyOnlySelection(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	homes := map[string]string{"main": "/synthetic/main", "second": "/synthetic/second", "third": "/synthetic/third"}
	usage := &transitionUsage043714{data: map[string]quotamodel.Usage{
		homes["main"]:   transitionUsageValue043714(10, 20, false, now),
		homes["second"]: transitionUsageValue043714(10, 20, false, now),
		homes["third"]:  transitionUsageValue043714(10, 20, false, now),
	}}
	store := transitionQuotaAccounts043714{homes: homes}
	status := quotausecase.NewStatus(store, usage)
	defer status.Close()
	accounts := []proxymodel.Account{
		{ID: "main", Home: homes["main"], Enabled: true, RouteOrder: 1},
		{ID: "second", Home: homes["second"], Enabled: true, RouteOrder: 2},
		{ID: "third", Home: homes["third"], Enabled: true, RouteOrder: 3},
	}
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-6.1-sol"}
	for _, account := range accounts {
		if _, err := status.AvailabilityForRoute(t.Context(), accountentity.Account{ID: account.ID, Enabled: true}, selection); err != nil {
			t.Fatal(err)
		}
	}
	gateway := &transitionGateway043714{}
	router, err := NewRouter(Config{Gateway: gateway, QuotaPreflight: status, PreferredAccount: "main",
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	send := func(want string, route quotamodel.RouteKind) {
		t.Helper()
		response, err := router.Forward(t.Context(), proxymodel.Request{QuotaSelection: quotamodel.Selection{RouteKind: route, RequestedModel: selection.RequestedModel}})
		if err != nil {
			t.Fatal(err)
		}
		if response.Result.AccountID != want || response.Result.Response.StatusCode != 200 {
			t.Fatalf("expected %s, got %s, calls=%v", want, response.Result.AccountID, gateway.calls)
		}
		if err := response.Close(); err != nil {
			t.Fatal(err)
		}
	}
	refresh := func(names ...string) {
		t.Helper()
		before := status.ProbeRefreshRevision()
		for _, name := range names {
			if err := status.ScheduleProbeRefresh(t.Context(), accountentity.Account{ID: name, Enabled: true}, ""); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		for status.ProbeRefreshRevision()-before < uint64(len(names)) {
			old := status.ProbeRefreshRevision()
			if !status.WaitProbeRefresh(ctx, old) {
				t.Fatalf("probe revalidation stalled: revision=%d before=%d", status.ProbeRefreshRevision(), before)
			}
		}
	}
	send("main", quotamodel.RouteKindResponses)
	usage.set(homes["main"], transitionUsageValue043714(26, 100, false, now))
	usage.set(homes["second"], transitionUsageValue043714(100, 20, false, now))
	usage.set(homes["third"], transitionUsageValue043714(0, 54, true, now))
	refresh("main", "second", "third")
	// Tagged Prodex 0.437.14 seed: the Pro Lite owner has a *historical*
	// authentication failure even though its weekly quota is ready.
	beforeAuth := status.ProbeRefreshRevision()
	authDigest := [32]byte{7, 1, 4}
	router.recordRouteOutcome(t.Context(), "third", selection, &proxymodel.Response{
		StatusCode: 401, AuthCredentialKnown: true, AuthCredentialDigest: authDigest,
		AuthRequestStartedAt: time.Now().Add(-6 * time.Second),
	}, responseOutcome{kind: responseAuthFailure})
	router.mu.Lock()
	history := router.quarantine["third"]
	history.authFailedAt = time.Now().Add(-3 * time.Second)
	router.quarantine["third"] = history
	router.mu.Unlock()
	router.applyRetryOutcomeForAccount(t.Context(), accounts[2], selection, responseOutcome{kind: responseAuthFailure})
	authCtx, authCancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer authCancel()
	for status.ProbeRefreshRevision() == beforeAuth {
		observed := status.ProbeRefreshRevision()
		if !status.WaitProbeRefresh(authCtx, observed) {
			t.Fatal("authenticated Pro Lite revalidation never completed")
		}
	}
	send("third", quotamodel.RouteKindResponses)
	send("third", quotamodel.RouteKindCompact)
	usage.set(homes["third"], transitionUsageValue043714(0, 100, true, now))
	usage.set(homes["second"], transitionUsageValue043714(10, 20, false, now))
	refresh("third", "second")
	send("second", quotamodel.RouteKindResponses)
	usage.set(homes["second"], transitionUsageValue043714(100, 20, false, now))
	usage.set(homes["main"], transitionUsageValue043714(10, 20, false, now))
	refresh("second", "main")
	send("main", quotamodel.RouteKindResponses)
	if got := strings.Join(gateway.calls, ","); got != "main,third,third,second,main" {
		t.Fatalf("Prodex 0.437.14 three-profile quota transition changed order or duplicated upstream: %s", got)
	}
}
