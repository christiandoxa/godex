package routing

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type fakeRoutingRedeemer struct {
	accountID string
	redeemed  bool
	calls     int
	preferred []string
	poolIDs   [][]string
}

func (fake *fakeRoutingRedeemer) Try(
	_ context.Context,
	accounts []proxymodel.Account,
	preferredID string,
	_ proxymodel.Request,
) (string, bool, error) {
	fake.calls++
	fake.preferred = append(fake.preferred, preferredID)
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	fake.poolIDs = append(fake.poolIDs, ids)
	return fake.accountID, fake.redeemed, nil
}

type sequenceRoutingGateway struct {
	responses map[string][]routingResponseFixture
	calls     []string
}

type routingResponseFixture struct {
	status int
	body   string
	header http.Header
}

func (gateway *sequenceRoutingGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	fixtures := gateway.responses[account.ID]
	if len(fixtures) == 0 {
		return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}
	fixture := fixtures[0]
	if len(fixtures) > 1 {
		gateway.responses[account.ID] = fixtures[1:]
	}
	header := fixture.header
	if header == nil {
		header = make(http.Header)
	}
	return &proxymodel.Response{StatusCode: fixture.status, Header: header, Body: io.NopCloser(strings.NewReader(fixture.body))}, nil
}

func TestAutoRedeemRunsWhenAllOpenAIAccountsArePreflightExhausted(t *testing.T) {
	now := time.Unix(100, 0)
	accounts := []proxymodel.Account{
		{ID: "a", Home: "/a", Enabled: true, EligibleAfter: now.Add(time.Hour)},
		{ID: "b", Home: "/b", Enabled: true, EligibleAfter: now.Add(time.Hour)},
	}
	gateway := &sequenceRoutingGateway{}
	redeemer := &fakeRoutingRedeemer{accountID: "b", redeemed: true}
	router, err := NewRouter(Config{
		Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		PreferredAccount: "a", Now: func() time.Time { return now }, AutoRedeem: true, Redeemer: redeemer,
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(context.Background(), proxymodel.Request{Body: []byte(`{"model":"gpt-5.3-codex"}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "b" || strings.Join(gateway.calls, ",") != "b" || redeemer.calls != 1 || redeemer.preferred[0] != "" {
		t.Fatalf("result/calls/redeemer = %#v / %v / %#v", exchange.Result, gateway.calls, redeemer)
	}
}

func TestAutoRedeemRunsAfterStructuredQuotaFailuresAndRetriesOnce(t *testing.T) {
	accounts := []proxymodel.Account{
		{ID: "a", Home: "/a", Enabled: true},
		{ID: "b", Home: "/b", Enabled: true},
	}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"a": {
			{status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`},
			{status: http.StatusOK, body: `{"id":"ok"}`},
		},
		"b": {{status: http.StatusForbidden, body: `{"error":{"code":"usage_limit_reached"}}`}},
	}}
	redeemer := &fakeRoutingRedeemer{accountID: "a", redeemed: true}
	router, err := NewRouter(Config{
		Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		PreferredAccount: "a", AutoRedeem: true, Redeemer: redeemer, MaxInspectBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "a,a" {
		t.Fatalf("gateway calls = %q", got)
	}
	if exchange.Result.AccountID != "a" || exchange.Result.Response.StatusCode != http.StatusOK || redeemer.calls != 1 {
		t.Fatalf("exchange/redeemer = %#v / %#v", exchange.Result, redeemer)
	}
	if len(redeemer.preferred) != 1 || redeemer.preferred[0] != "a" {
		t.Fatalf("quota-blocked redeem preference = %#v", redeemer.preferred)
	}
}

func TestAutoRedeemHardAffinityRetriesSameOwnerOnce(t *testing.T) {
	accounts := []proxymodel.Account{{ID: "owner", Home: "/owner", Enabled: true}}
	gateway := &sequenceRoutingGateway{responses: map[string][]routingResponseFixture{
		"owner": {
			{status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`},
			{status: http.StatusOK, body: `{"id":"ok"}`},
		},
	}}
	redeemer := &fakeRoutingRedeemer{accountID: "owner", redeemed: true}
	router, err := NewRouter(Config{
		Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		AutoRedeem: true, Redeemer: redeemer, MaxInspectBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := router.forwardBound(context.Background(), proxymodel.Request{}, accounts, "owner")
	if err != nil {
		t.Fatal(err)
	}
	defer result.Response.Body.Close()
	if got := strings.Join(gateway.calls, ","); got != "owner,owner" || result.AccountID != "owner" || result.Response.StatusCode != http.StatusOK {
		t.Fatalf("calls/result = %q / %#v", got, result)
	}
	if redeemer.calls != 1 || len(redeemer.preferred) != 1 || redeemer.preferred[0] != "owner" {
		t.Fatalf("redeemer = %#v", redeemer)
	}
}

func TestAutoRedeemDoesNotTreatTransientQuarantineAsCreditCandidate(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	accounts := []proxymodel.Account{
		{ID: "transient", Home: "/transient", Enabled: true},
		{ID: "quota", Home: "/quota", Enabled: true, EligibleAfter: now.Add(time.Hour)},
	}
	redeemer := &fakeRoutingRedeemer{accountID: "quota", redeemed: true}
	router, err := NewRouter(Config{
		Gateway: &sequenceRoutingGateway{}, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
		Now: func() time.Time { return now }, AutoRedeem: true, Redeemer: redeemer,
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.quarantineAccount("transient", time.Minute)
	exchange, err := router.Forward(context.Background(), proxymodel.Request{})
	if err != nil {
		t.Fatal(err)
	}
	exchange.Close()
	if len(redeemer.poolIDs) != 1 || strings.Join(redeemer.poolIDs[0], ",") != "quota" {
		t.Fatalf("auto-redeem pool = %#v", redeemer.poolIDs)
	}
	if len(waits) != 1 || waits[0] != 30*time.Second {
		t.Fatalf("transient recovery waits = %v", waits)
	}
}

func TestQuotaOutcomeMarksAndSuccessfulRetryClearsQuotaMarker(t *testing.T) {
	router := &Router{now: time.Now, quarantine: make(map[string]quarantineState), quotaBlocked: make(map[string]bool)}
	router.applyRetryOutcome("a", responseOutcome{kind: responseRetry, quarantine: time.Second, quota: true})
	if !router.quotaBlockedAccount("a") {
		t.Fatal("quota marker was not set")
	}
	router.clearQuarantine("a")
	if router.quotaBlockedAccount("a") || router.isQuarantined("a", time.Now()) {
		t.Fatal("quota/quarantine marker survived clear")
	}
}
