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

func prodex04356WebSocketFailure(code string, transport bool) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode:      http.StatusOK,
		Header:          make(http.Header),
		Body:            io.NopCloser(strings.NewReader("retryable-failure")),
		WebSocketFrames: true,
		PrecommitFailure: &proxymodel.PrecommitFailure{
			Code:      code,
			Transport: transport,
		},
	}
}

func prodex04356WebSocketSuccess(responseID string) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode:          http.StatusOK,
		Header:              make(http.Header),
		Body:                io.NopCloser(strings.NewReader("success-frame")),
		WebSocketFrames:     true,
		FirstEventCommitted: true,
		WebSocketResponseID: responseID,
	}
}

func TestProdex04356WebSocketHardAffinityRetryableFailuresSignalFullContextWithoutSession(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		transport bool
	}{
		{name: "quota", code: "insufficient_quota"},
		{name: "usage not included", code: "usage_not_included"},
		{name: "rate limit", code: "rate_limit_exceeded"},
		{name: "overload", code: "server_is_overloaded"},
		{name: "auth", code: "unauthorized"},
		{name: "transport", code: "transport_failed", transport: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
				prodex04356WebSocketFailure(test.code, test.transport),
				prodex04356WebSocketSuccess("resp-replayed"),
			}}
			router := newWebSocketMessageRouter(t, gateway)
			if err := router.affinity.remember(
				t.Context(), "account-a", affinityKeys{previous: "resp-owner"}, router.now(),
			); err != nil {
				t.Fatal(err)
			}

			continuation, err := router.Forward(t.Context(), websocketMessageRequest(
				`{"type":"response.create","previous_response_id":"resp-owner","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`,
			))
			if err != nil {
				t.Fatal(err)
			}
			payload, readErr := io.ReadAll(continuation.Result.Response.Body)
			if readErr != nil {
				t.Fatal(readErr)
			}
			_ = continuation.Close()
			owner, ownerErr := router.affinity.owner(
				t.Context(), affinityKeys{previous: "resp-owner"}, router.now(),
			)
			if ownerErr != nil {
				t.Fatal(ownerErr)
			}
			if got := strings.Join(gateway.accounts, ","); got != "account-a" ||
				continuation.Result.Response.StatusCode != http.StatusBadRequest ||
				!strings.Contains(string(payload), "previous_response_not_found") ||
				!strings.Contains(string(payload), "Previous response was not found. Retrying the full request.") ||
				owner != "" {
				t.Fatalf("signal calls/status/body/owner = %s/%d/%s/%q",
					got, continuation.Result.Response.StatusCode, payload, owner)
			}

			replay, err := router.Forward(t.Context(), websocketMessageRequest(
				`{"type":"response.create","input":[{"role":"user","content":"original"},{"role":"assistant","content":"answer"},{"role":"user","content":"continue"}]}`,
			))
			if err != nil {
				t.Fatal(err)
			}
			defer replay.Close()
			if got := strings.Join(gateway.accounts, ","); got != "account-a,account-b" ||
				replay.Result.AccountID != "account-b" || replay.Result.Response.StatusCode != http.StatusOK {
				t.Fatalf("replay calls/owner/status = %s/%s/%d",
					got, replay.Result.AccountID, replay.Result.Response.StatusCode)
			}
		})
	}
}

func TestProdex04356WebSocketHardAffinityRetryableFailureWithoutFallbackStaysTerminal(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		prodex04356WebSocketFailure("server_is_overloaded", false),
	}}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "synthetic-a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.remember(
		t.Context(), "account-a", affinityKeys{previous: "resp-owner"}, router.now(),
	); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), websocketMessageRequest(
		`{"type":"response.create","previous_response_id":"resp-owner","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	owner, ownerErr := router.affinity.owner(
		t.Context(), affinityKeys{previous: "resp-owner"}, router.now(),
	)
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if got := strings.Join(gateway.accounts, ","); got != "account-a" ||
		exchange.Result.Response.StatusCode != http.StatusOK ||
		exchange.Result.Response.PrecommitFailure == nil ||
		exchange.Result.Response.PrecommitFailure.Code != "server_is_overloaded" ||
		owner != "account-a" {
		t.Fatalf("terminal calls/status/failure/owner = %s/%d/%#v/%q",
			got, exchange.Result.Response.StatusCode, exchange.Result.Response.PrecommitFailure, owner)
	}
}

func TestProdex04356WebSocketBoundSessionRetryableFailureRotatesAndRebinds(t *testing.T) {
	gateway := &websocketMessageRoutingGateway{responses: []*proxymodel.Response{
		prodex04356WebSocketFailure("server_is_overloaded", false),
		prodex04356WebSocketSuccess("resp-new"),
	}}
	router := newWebSocketMessageRouter(t, gateway)
	if err := router.affinity.remember(
		t.Context(), "account-a", affinityKeys{session: "sess-bound"}, router.now(),
	); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), websocketMessageRequest(
		`{"type":"response.create","session_id":"sess-bound","input":[{"role":"user","content":"hello"}]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	owner, ownerErr := router.affinity.owner(
		t.Context(), affinityKeys{session: "sess-bound"}, router.now(),
	)
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if got := strings.Join(gateway.accounts, ","); got != "account-a,account-b" ||
		exchange.Result.AccountID != "account-b" || owner != "account-b" {
		t.Fatalf("session recovery calls/result/owner = %s/%s/%s", got, exchange.Result.AccountID, owner)
	}
}

func TestProdex04356WebSocketPreSendQuotaHardAffinityWithoutSessionSignalsReplay(t *testing.T) {
	now := time.Unix(8_000_000, 0)
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "synthetic-a", Enabled: true, EligibleAfter: now.Add(time.Hour)},
		{ID: "account-b", Home: "synthetic-b", Enabled: true},
	}
	gateway := &websocketMessageRoutingGateway{}
	router, err := NewRouter(Config{
		Gateway: gateway,
		Now:     func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.affinity.remember(
		t.Context(), "account-a", affinityKeys{previous: "resp-owner"}, now,
	); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), websocketMessageRequest(
		`{"type":"response.create","previous_response_id":"resp-owner","input":[{"type":"function_call_output","call_id":"call-1","output":"done"}]}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	payload, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	owner, ownerErr := router.affinity.owner(t.Context(), affinityKeys{previous: "resp-owner"}, now)
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	if len(gateway.accounts) != 0 ||
		exchange.Result.Response.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(payload), "previous_response_not_found") ||
		owner != "" {
		t.Fatalf("pre-send calls/status/body/owner = %v/%d/%s/%q",
			gateway.accounts, exchange.Result.Response.StatusCode, payload, owner)
	}
}
