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

type prodex04354CompactGateway struct {
	calls []string
}

func (gateway *prodex04354CompactGateway) Execute(
	_ context.Context,
	_ proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.calls = append(gateway.calls, account.ID)
	status := http.StatusOK
	body := "{}"
	switch len(gateway.calls) {
	case 1:
		status = http.StatusServiceUnavailable
		body = `{"error":{"code":"temporarily_unavailable"}}`
	case 2:
		status = http.StatusForbidden
		body = `{"error":{"code":"insufficient_quota"}}`
	default:
		body = `{"id":"must-not-retry-excluded"}`
	}
	return &proxymodel.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestProdex04354CompactQuotaFallbackHonorsRequestLocalExclusions(t *testing.T) {
	now := time.Unix(100, 0)
	var waits []time.Duration
	gateway := &prodex04354CompactGateway{}
	router, err := NewRouter(Config{
		Gateway:          gateway,
		PreferredAccount: "profile-b",
		Now:              func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "profile-a", Home: "/a", Enabled: true},
				{ID: "profile-b", Home: "/b", Enabled: true},
			}, nil
		},
		Wait: func(ctx context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			now = now.Add(delay)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(context.Background(), proxymodel.Request{
		Path:   "/backend-api/codex/responses/compact",
		Header: make(http.Header),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	body, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	forwarded := append(append([]byte(nil), exchange.Result.Prefix...), body...)
	if exchange.Result.AccountID != "profile-a" ||
		!exchange.Result.Failed ||
		exchange.Result.Response.StatusCode != http.StatusForbidden ||
		!strings.Contains(string(forwarded), "insufficient_quota") ||
		strings.Join(gateway.calls, ",") != "profile-b,profile-a" ||
		len(waits) != 0 {
		t.Fatalf("owner/failed/status/body/calls/waits = %q/%t/%d/%q/%v/%v",
			exchange.Result.AccountID, exchange.Result.Failed,
			exchange.Result.Response.StatusCode, forwarded, gateway.calls, waits)
	}
}

func TestProdex04354CompactExclusionRuleDoesNotAffectResponses(t *testing.T) {
	request := proxymodel.Request{Path: "/backend-api/codex/responses"}
	candidates := []proxymodel.Account{
		{ID: "profile-a", Home: "/a", Enabled: true},
		{ID: "profile-b", Home: "/b", Enabled: true},
	}
	excluded := map[string]bool{"profile-b": true}
	if compactQuotaFallbackExhaustedForRequest(request, candidates, "profile-a", excluded) {
		t.Fatal("standard Responses incorrectly used Compact request-local fallback exhaustion")
	}
}
