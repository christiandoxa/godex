package routing

import (
	"fmt"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProxyQuarantineIsBounded(t *testing.T) {
	proxy := &Router{now: func() time.Time { return time.Unix(10, 0) }, quarantine: make(map[string]time.Time)}
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
	proxy := &Router{now: func() time.Time { return time.Unix(10, 0) }, quarantine: make(map[string]time.Time)}
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
		{name: "rate limit", status: http.StatusTooManyRequests, kind: responseRetry, quarantine: true},
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
			outcome, pending := proxy.classify(response)
			if outcome.kind != testCase.kind || (outcome.quarantine > 0) != testCase.quarantine {
				t.Fatalf("outcome = %#v, pending = %#v", outcome, pending)
			}
			if pending != nil {
				pending.close()
			}
		})
	}
}
