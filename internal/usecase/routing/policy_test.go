package routing

import (
	"context"
	"fmt"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProxyQuarantineIsBounded(t *testing.T) {
	proxy := &Router{now: func() time.Time { return time.Unix(10, 0) }, quarantine: make(map[string]quarantineState)}
	for index := 0; index <= maxQuarantinedAccounts; index++ {
		proxy.quarantineAccount(fmt.Sprintf("account-%d", index), time.Minute)
	}
	proxy.mu.Lock()
	count := len(proxy.quarantine)
	proxy.mu.Unlock()
	if count != maxQuarantinedAccounts {
		t.Fatalf("quarantine entries = %d, want %d", count, maxQuarantinedAccounts)
	}
}

func TestProxyQuarantineKeepsLongerExistingLease(t *testing.T) {
	proxy := &Router{now: func() time.Time { return time.Unix(10, 0) }, quarantine: make(map[string]quarantineState)}
	proxy.quarantineAccount("synthetic", time.Hour)
	proxy.quarantineAccount("synthetic", time.Second)
	if !proxy.isQuarantined("synthetic", time.Unix(10, 0).Add(time.Minute)) {
		t.Fatal("longer quarantine lease was shortened")
	}
}

func TestSortRuntimeAccountsDeterministicallyDeduplicates(t *testing.T) {
	accounts := sortRuntimeAccounts([]proxymodel.Account{
		{ID: "same", Home: "/z", Enabled: false},
		{ID: "same", Home: "/a", Enabled: true},
	})
	if len(accounts) != 1 || accounts[0].Home != "/a" || !accounts[0].Enabled {
		t.Fatalf("sorted accounts = %#v", accounts)
	}
}

func TestSortRuntimeAccountsUsesHomeAsTieBreaker(t *testing.T) {
	accounts := sortRuntimeAccounts([]proxymodel.Account{
		{ID: "same", Home: "/z", Enabled: true},
		{ID: "same", Home: "/a", Enabled: true},
	})
	if len(accounts) != 1 || accounts[0].Home != "/a" {
		t.Fatalf("sorted accounts = %#v", accounts)
	}
}

func TestClassifyPreCommitFailures(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		kind       responseKind
		quarantine bool
	}{
		{name: "server error", status: http.StatusBadGateway, kind: responseRetry},
		{name: "rate limit", status: http.StatusTooManyRequests, body: `{"error":{"code":"rate_limit_exceeded"}}`, kind: responseRetry, quarantine: true},
		{name: "generic rate limit", status: http.StatusTooManyRequests, kind: responseRetry, quarantine: true},
		{name: "unauthorized", status: http.StatusUnauthorized, kind: responseAuthFailure},
		{name: "client error", status: http.StatusBadRequest, body: `{"error":{"code":"invalid_request"}}`, kind: responsePass},
		{name: "quota body", status: http.StatusForbidden, body: `{"error":{"code":"insufficient_quota"}}`, kind: responseRetry, quarantine: true},
	}
	proxy := &Router{now: func() time.Time { return time.Unix(10, 0) }, maxInspect: 1024}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := &proxymodel.Response{
				StatusCode: testCase.status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(testCase.body)),
			}
			if testCase.name == "rate limit" {
				response.Header.Set("Retry-After", "5")
			}
			outcome, pending, err := proxy.classify(response, "")
			if err != nil {
				t.Fatal(err)
			}
			if outcome.kind != testCase.kind || (outcome.quarantine > 0) != testCase.quarantine {
				t.Fatalf("outcome = %#v, pending = %#v", outcome, pending)
			}
			if pending != nil {
				pending.close()
			}
		})
	}
}

func TestLaunchQuotaExclusionRecoversAtItsDeadline(t *testing.T) {
	now := time.Unix(100, 0)
	router := &Router{now: func() time.Time { return now }, quarantine: make(map[string]quarantineState)}
	accounts := []proxymodel.Account{{ID: "exhausted", Home: "home", Enabled: true, EligibleAfter: now.Add(time.Minute)}}
	if got := router.candidates(accounts, now); len(got) != 0 {
		t.Fatal("exhausted account admitted early")
	}
	now = now.Add(time.Minute)
	if got := router.candidates(accounts, now); len(got) != 1 {
		t.Fatal("quota account remained disabled after reset")
	}
	accounts[0].Enabled = false
	if got := router.candidates(accounts, now); len(got) != 0 {
		t.Fatal("business-disabled account admitted at quota reset")
	}
}

func TestExternalProviderClassifyUsesStructured429Policy(t *testing.T) {
	proxy := &Router{now: func() time.Time { return time.Unix(10, 0) }, maxInspect: 1024}
	fixtures := []struct {
		name   string
		status int
		body   string
		kind   responseKind
	}{
		{"bare 429", http.StatusTooManyRequests, "{\"error\":{\"message\":\"too many requests\"}}", responsePass},
		{"rate 429", http.StatusTooManyRequests, "{\"error\":{\"code\":\"rate_limit_exceeded\"}}", responseRetry},
		{"quota 429", http.StatusTooManyRequests, "{\"error\":{\"type\":\"quota_exhausted\"}}", responseRetry},
		{"forbidden auth", http.StatusForbidden, "{\"error\":{\"code\":\"quota_exhausted\"}}", responseAuthFailure},
		{"not found terminal", http.StatusNotFound, "{\"error\":{\"code\":\"model_not_supported\"}}", responsePass},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			response := &proxymodel.Response{
				StatusCode: fixture.status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(fixture.body)),
			}
			outcome, pending, err := proxy.classify(response, "anthropic")
			if err != nil {
				t.Fatal(err)
			}
			if outcome.kind != fixture.kind {
				t.Fatalf("outcome = %#v", outcome)
			}
			if pending == nil || string(pending.prefix) != fixture.body {
				t.Fatalf("pending prefix = %#v", pending)
			}
			pending.close()
		})
	}
}

type externalRetryGateway struct {
	responses map[string]struct {
		status int
		body   string
	}
	calls []string
}

func (gateway *externalRetryGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	fixture := gateway.responses[account.ID]
	return &proxymodel.Response{
		StatusCode: fixture.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(fixture.body)),
	}, nil
}

func TestExternalProviderRoutingRotatesOnlyStructuredRetryableFailures(t *testing.T) {
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, Provider: proxymodel.Provider{Kind: "anthropic"}},
		{ID: "account-b", Home: "/b", Enabled: true, Provider: proxymodel.Provider{Kind: "anthropic"}},
	}
	cases := []struct {
		name      string
		firstCode int
		firstBody string
		wantCalls string
		wantCode  int
	}{
		{"bare 429 stays terminal", http.StatusTooManyRequests, `{"error":{"message":"too many requests"}}`, "account-a", http.StatusTooManyRequests},
		{"quota rotates", http.StatusTooManyRequests, `{"error":{"code":"quota_exhausted"}}`, "account-a,account-b", http.StatusOK},
		{"auth rotates", http.StatusUnauthorized, `{"error":{"type":"authentication_error"}}`, "account-a,account-b", http.StatusOK},
		{"not found stays terminal", http.StatusNotFound, `{"error":{"code":"model_not_supported"}}`, "account-a", http.StatusNotFound},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &externalRetryGateway{responses: map[string]struct {
				status int
				body   string
			}{
				"account-a": {fixture.firstCode, fixture.firstBody},
				"account-b": {http.StatusOK, `{}`},
			}}
			router, err := NewRouter(Config{
				Gateway: gateway,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return accounts, nil
				},
				PreferredAccount: "account-a",
				MaxInspectBytes:  1024,
			})
			if err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header)})
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			if got := strings.Join(gateway.calls, ","); got != fixture.wantCalls {
				t.Fatalf("calls = %q, want %q", got, fixture.wantCalls)
			}
			if exchange.Result.Response.StatusCode != fixture.wantCode {
				t.Fatalf("status = %d, want %d", exchange.Result.Response.StatusCode, fixture.wantCode)
			}
		})
	}
}

func TestSortRuntimeAccountsPreservesExplicitProviderRouteOrder(t *testing.T) {
	accounts := sortRuntimeAccounts([]proxymodel.Account{
		{ID: "hash-a", Home: "/third", Enabled: true, RouteOrder: 3},
		{ID: "hash-z", Home: "/first", Enabled: true, RouteOrder: 1},
		{ID: "hash-m", Home: "/second", Enabled: true, RouteOrder: 2},
	})
	if len(accounts) != 3 ||
		accounts[0].Home != "/first" ||
		accounts[1].Home != "/second" ||
		accounts[2].Home != "/third" {
		t.Fatalf("provider route order = %#v", accounts)
	}
}

func TestProdex04356CopilotGeneric429RotatesCredentialBeforeCommit(t *testing.T) {
	proxy := &Router{now: func() time.Time { return time.Unix(10, 0) }, maxInspect: 1024}
	for _, fixture := range []struct {
		name string
		body string
		kind responseKind
	}{
		{name: "generic", body: `{"error":{"message":"too many requests"}}`, kind: responseRetry},
		{name: "invalid request", body: `{"error":{"type":"invalid_request_error"}}`, kind: responsePass},
		{name: "model not supported", body: `{"error":{"code":"model_not_supported"}}`, kind: responsePass},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			response := &proxymodel.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(fixture.body)),
			}
			outcome, pending, err := proxy.classify(response, "copilot")
			if err != nil {
				t.Fatal(err)
			}
			defer pending.close()
			if outcome.kind != fixture.kind {
				t.Fatalf("outcome = %#v, want kind %d", outcome, fixture.kind)
			}
		})
	}

	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/a", Enabled: true, Provider: proxymodel.Provider{Kind: "copilot"}},
		{ID: "account-b", Home: "/b", Enabled: true, Provider: proxymodel.Provider{Kind: "copilot"}},
	}
	gateway := &externalRetryGateway{responses: map[string]struct {
		status int
		body   string
	}{
		"account-a": {http.StatusTooManyRequests, `{"error":{"message":"too many requests"}}`},
		"account-b": {http.StatusOK, `{}`},
	}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return accounts, nil
		},
		PreferredAccount: "account-a",
		MaxInspectBytes:  1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), proxymodel.Request{Header: make(http.Header)})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if got := strings.Join(gateway.calls, ","); got != "account-a,account-b" ||
		exchange.Result.AccountID != "account-b" ||
		exchange.Result.Response.StatusCode != http.StatusOK {
		t.Fatalf("Copilot generic 429 calls/owner/status = %s/%s/%d",
			got, exchange.Result.AccountID, exchange.Result.Response.StatusCode)
	}
}
