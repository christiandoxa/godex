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

// Prodex 0.436.1 returns the original 429 when one configured provider key
// has neither an alternate model nor a second credential to rotate to.
func TestProdex04361SingleExternalCredential429IsTerminal(t *testing.T) {
	for _, fixture := range []struct {
		name string
		body string
	}{
		{name: "structured_rate", body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{name: "structured_quota", body: `{"error":{"code":"quota_exhausted"}}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &externalRetryGateway{responses: map[string]struct {
				status int
				body   string
			}{"single-key": {http.StatusTooManyRequests, fixture.body}}}
			router, err := NewRouter(Config{
				Gateway: gateway,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{
						ID: "single-key", Home: "/synthetic", Enabled: true,
						Provider: proxymodel.Provider{Kind: "deepseek"},
					}}, nil
				},
				PreferredAccount: "single-key",
				MaxInspectBytes:  1024,
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
			defer cancel()
			exchange, err := router.Forward(ctx, proxymodel.Request{Header: make(http.Header)})
			if err != nil {
				t.Fatalf("single key incorrectly entered retry wait: %v", err)
			}
			defer exchange.Close()
			if len(gateway.calls) != 1 || gateway.calls[0] != "single-key" {
				t.Fatalf("unexpected attempt count: %v", gateway.calls)
			}
			if exchange.Result.Response.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("status = %d; want 429", exchange.Result.Response.StatusCode)
			}
			body, err := io.ReadAll(exchange.Result.Response.Body)
			payload := append(append([]byte(nil), exchange.Result.Prefix...), body...)
			if err != nil || strings.TrimSpace(string(payload)) != fixture.body {
				t.Fatalf("429 body changed: %q, err=%v", payload, err)
			}
		})
	}
}

// A terminal 429 must not silently poison the only key's route eligibility.
// Prodex permits a fresh later client request to use the same key again.
type singleExternal429ThenOK struct{ calls int }

func (fake *singleExternal429ThenOK) Execute(
	_ context.Context, _ proxymodel.Request, _ proxymodel.Account,
) (*proxymodel.Response, error) {
	fake.calls++
	status, payload := http.StatusOK, `{"ok":true}`
	if fake.calls == 1 {
		status, payload = http.StatusTooManyRequests, `{"error":{"code":"rate_limit_exceeded"}}`
	}
	return &proxymodel.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func TestProdex04361SingleExternal429DoesNotBlockNextIndependentTurn(t *testing.T) {
	gateway := &singleExternal429ThenOK{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "single", Home: "/synthetic", Enabled: true, Provider: proxymodel.Provider{Kind: "deepseek"}}}, nil
		},
		PreferredAccount: "single",
		MaxInspectBytes:  1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for index, expected := range []int{http.StatusTooManyRequests, http.StatusOK} {
		exchange, err := router.Forward(ctx, proxymodel.Request{Header: make(http.Header)})
		if err != nil {
			t.Fatalf("turn %d: %v", index+1, err)
		}
		if exchange.Result.Response.StatusCode != expected {
			t.Fatalf("turn %d returned %d, want %d", index+1, exchange.Result.Response.StatusCode, expected)
		}
		if err := exchange.Close(); err != nil {
			t.Fatalf("turn %d close: %v", index+1, err)
		}
	}
	if gateway.calls != 2 {
		t.Fatalf("one upstream execution per client turn expected, got %d", gateway.calls)
	}
}
