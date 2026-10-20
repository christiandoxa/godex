package routing

import (
	"context"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Prodex's tagged parse_runtime_sse_payload returns None if serde_json
// rejects the *entire* event. Its default inspection event has no error
// signal or metadata-hold kind, so the lookahead commits the opaque event,
// rather than treating malformed response.failed JSON as a quota/429 error.
func TestProdex04383MalformedSSEEventCannotForgePrecommitErrorSignals(t *testing.T) {
	validRate := `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded"}}}`
	validQuota := `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}`
	for _, tc := range []struct {
		name, payload string
		wantRetry     bool
	}{
		{"valid-rate", validRate, true},
		{"valid-quota", validQuota, true},
		{"lone-high", strings.TrimSuffix(validQuota, "}") + `,"note":"\ud800"}`, false},
		{"lone-low", strings.TrimSuffix(validRate, "}") + `,"note":"\udc00"}`, false},
		{"nonfinite-float", strings.TrimSuffix(validQuota, "}") + `,"note":1e309}`, false},
		{"overdepth", strings.TrimSuffix(validQuota, "}") + `,"note":` + strings.Repeat(`{"a":`, 128) + `0` + strings.Repeat(`}`, 128) + `}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcome, wait := streamOutcome([]byte(tc.payload), make(http.Header), time.Unix(2_000_000_000, 0), "openai")
			if tc.wantRetry {
				if wait || outcome.kind != responseRetry {
					t.Fatalf("valid precommit retry lost: outcome=%+v wait=%v", outcome, wait)
				}
			} else if wait || outcome.kind != responsePass || outcome.quota || outcome.firstEventRetry {
				t.Fatalf("malformed SSE incorrectly influences precommit: outcome=%+v wait=%v", outcome, wait)
			}
		})
	}
}

// An upstream malformed SSE error cannot consume the retry/rotation budget.
// The client must receive the original event on the same Ready profile.
func TestProdex04383MalformedSSEPrecommitCannotRotateOwner(t *testing.T) {
	for _, tc := range []struct{ name, note string }{
		{"unpaired-surrogate", `"\ud800"`},
		{"overflow", `1e309`},
		{"overdepth", strings.Repeat(`{"a":`, 128) + `0` + strings.Repeat(`}`, 128)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			malformed := `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}},"note":` + tc.note + `}`
			wire := "data: " + malformed + "\n\n"
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusOK, "text/event-stream", wire},
				{http.StatusOK, "text/event-stream", "data: {\"type\":\"response.completed\"}\n\n"},
			}}
			router, err := NewRouter(Config{Gateway: gateway, PreferredAccount: "account-a", Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{
					{ID: "account-a", Home: "/synthetic/a", Enabled: true}, {ID: "account-b", Home: "/synthetic/b", Enabled: true},
				}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header), Body: []byte(`{"model":"gpt-6","stream":true,"input":[]}`), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			forwarded, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer forwarded.Close()
			payload, err := io.ReadAll(forwarded.Result.Response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if forwarded.Result.AccountID != "account-a" || len(gateway.accounts) != 1 || gateway.accounts[0] != "account-a" || string(forwarded.Result.Prefix)+string(payload) != wire {
				t.Fatalf("malformed SSE rotated or altered output: owner=%q attempts=%v payload=%s", forwarded.Result.AccountID, gateway.accounts, string(forwarded.Result.Prefix)+string(payload))
			}
		})
	}
}
