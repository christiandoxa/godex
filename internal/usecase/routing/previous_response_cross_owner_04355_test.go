package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Tagged 0.438.3 fails closed when the verified previous-response owner
// rejects a chain. A second managed account cannot safely replay it.
func TestProdex04383ResponsesPreviousNotFoundFailsClosedWithoutCrossOwnerReplay(t *testing.T) {
	const body = `{"previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"continue"}],"client_metadata":{"session_id":"session-a"}}`
	const notFound = `{"error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"previous response not found"}}`
	now := time.Unix(5_000_000, 0)
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{status: http.StatusBadRequest, contentType: "application/json", body: notFound},
		{status: http.StatusOK, contentType: "application/json", body: `{"id":"resp-new"}`},
	}}
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/synthetic/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/synthetic/b", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := responsesRecoveryRequest(body)
	keys := requestAffinity(request, request.Body)
	if err := router.affinity.remember(t.Context(), "account-a", keys, now); err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" || !exchange.Result.Failed || exchange.Result.Response.StatusCode != http.StatusConflict {
		t.Fatalf("locked previous result = account %q failed=%t status=%d", exchange.Result.AccountID, exchange.Result.Failed, exchange.Result.Response.StatusCode)
	}
	if !reflect.DeepEqual(gateway.accounts, []string{"account-a"}) {
		t.Fatalf("verified response was replayed across owners: %v", gateway.accounts)
	}
	for index, upstream := range gateway.requests {
		if got := requestPreviousResponseID(upstream.Body); got != "resp-dead" {
			t.Fatalf("attempt %d previous_response_id = %q, want resp-dead", index, got)
		}
	}
	if score := router.previousResponseFailureScore("account-a", "resp-dead", request.QuotaSelection, now); score != 1 {
		t.Fatalf("owner negative-cache score = %d, want 1", score)
	}
	if owner, err := router.affinity.owner(t.Context(), affinityKeys{previous: "resp-dead"}, now); err != nil || owner != "account-a" {
		t.Fatalf("failed first recognition changed verified owner: %q err=%v", owner, err)
	}
}

func TestProdex04355ResponsesPreviousNotFoundPoolExhaustionReturnsStale409(t *testing.T) {
	const body = `{"previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"continue"}]}`
	const notFound = `{"error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"previous response not found"}}`
	now := time.Unix(5_100_000, 0)
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{{status: http.StatusBadRequest, contentType: "application/json", body: notFound}}}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/codex/responses",
		Header: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(body),
		QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
	}
	if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{previous: "resp-dead"}, now); err != nil {
		t.Fatal(err)
	}

	exchange, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.Response.StatusCode != http.StatusConflict {
		t.Fatalf("stale status = %d, want 409", exchange.Result.Response.StatusCode)
	}
	responseBody, err := io.ReadAll(exchange.Result.Response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if json.Unmarshal(responseBody, &value) != nil {
		t.Fatalf("stale response = %q", responseBody)
	}
	errorValue, _ := value["error"].(map[string]any)
	if errorValue["code"] != "stale_continuation" || !strings.Contains(errorValue["message"].(string), "Upstream no longer recognizes") || strings.Contains(string(responseBody), "previous_response_not_found") {
		t.Fatalf("stale response = %s", responseBody)
	}
	if !reflect.DeepEqual(gateway.accounts, []string{"account-a"}) {
		t.Fatalf("pool exhaustion attempts = %v", gateway.accounts)
	}
	if owner, err := router.affinity.owner(t.Context(), affinityKeys{previous: "resp-dead"}, now); err != nil || owner != "account-a" {
		t.Fatalf("durable owner after first miss = %q err=%v", owner, err)
	}
	if score := router.previousResponseFailureScore("account-a", "resp-dead", request.QuotaSelection, now); score != 1 {
		t.Fatalf("negative-cache score = %d, want 1", score)
	}
}

type responsesTurnStateRecoveryGateway struct {
	requests []proxymodel.Request
	accounts []string
}

func (gateway *responsesTurnStateRecoveryGateway) Execute(
	_ context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	index := len(gateway.requests)
	gateway.requests = append(gateway.requests, request)
	gateway.accounts = append(gateway.accounts, account.ID)
	if index < 4 {
		header := http.Header{"Content-Type": []string{"application/json"}}
		header.Set("X-Codex-Turn-State", "turn-state-"+string(rune('1'+index)))
		return &proxymodel.Response{
			StatusCode: http.StatusBadRequest,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"previous response not found"}}`)),
		}, nil
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp-new"}`)),
	}, nil
}

func TestProdex04383ResponsesPreviousNotFoundRetriesReturnedTurnStateThenFailsClosed(t *testing.T) {
	now := time.Unix(5_200_000, 0)
	gateway := &responsesTurnStateRecoveryGateway{}
	var waits []time.Duration
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/synthetic/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/synthetic/b", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now },
		Wait: func(_ context.Context, delay time.Duration) error { waits = append(waits, delay); return nil },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := responsesRecoveryRequest(`{"previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"continue"}]}`)
	keys := requestAffinity(request, request.Body)
	if err := router.affinity.remember(t.Context(), "account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" || !exchange.Result.Failed || exchange.Result.Response.StatusCode != http.StatusConflict {
		t.Fatalf("bounded turn-state recovery incorrectly replayed: account=%q failed=%t status=%d", exchange.Result.AccountID, exchange.Result.Failed, exchange.Result.Response.StatusCode)
	}
	wantWaits := []time.Duration{75 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond}
	if !reflect.DeepEqual(waits, wantWaits) {
		t.Fatalf("turn-state waits = %v, want %v", waits, wantWaits)
	}
	if !reflect.DeepEqual(gateway.accounts, []string{"account-a", "account-a", "account-a", "account-a"}) {
		t.Fatalf("turn-state attempts escaped verified owner: %v", gateway.accounts)
	}
	for index, want := range []string{"turn-state-1", "turn-state-2", "turn-state-3"} {
		if got := gateway.requests[index+1].Header.Get("x-codex-turn-state"); got != want {
			t.Fatalf("retry %d turn-state = %q, want %q", index, got, want)
		}
	}
	if score := router.previousResponseFailureScore("account-a", "resp-dead", request.QuotaSelection, now); score != 1 {
		t.Fatalf("negative cache after retry budget = %d, want 1", score)
	}
}

func TestProdex04383ResponsesPreviousNotFoundRejectsCrossOwnerOnMultipleReadyProfiles(t *testing.T) {
	const notFound = `{"error":{"type":"invalid_request_error","code":"previous_response_not_found","message":"previous response not found"}}`
	now := time.Unix(5_300_000, 0)
	gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
		{status: http.StatusBadRequest, contentType: "application/json", body: notFound},
		{status: http.StatusBadRequest, contentType: "application/json", body: notFound},
		{status: http.StatusOK, contentType: "application/json", body: `{"id":"resp-new"}`},
	}}
	accounts := []proxymodel.Account{
		{ID: "account-a", Home: "/synthetic/a", Enabled: true, RouteOrder: 1},
		{ID: "account-b", Home: "/synthetic/b", Enabled: true, RouteOrder: 2},
		{ID: "account-c", Home: "/synthetic/c", Enabled: true, RouteOrder: 3},
	}
	router, err := NewRouter(Config{
		Gateway: gateway, Now: func() time.Time { return now },
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := responsesRecoveryRequest(`{"previous_response_id":"resp-dead","input":[{"type":"message","role":"user","content":"continue"}]}`)
	keys := requestAffinity(request, request.Body)
	if err := router.affinity.remember(t.Context(), "account-a", keys, now); err != nil {
		t.Fatal(err)
	}
	exchange, err := router.Forward(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer exchange.Close()
	if exchange.Result.AccountID != "account-a" || !exchange.Result.Failed || exchange.Result.Response.StatusCode != http.StatusConflict {
		t.Fatalf("multi-owner previous response was not fail-closed: account=%q failed=%t status=%d", exchange.Result.AccountID, exchange.Result.Failed, exchange.Result.Response.StatusCode)
	}
	if !reflect.DeepEqual(gateway.accounts, []string{"account-a"}) {
		t.Fatalf("verified chain escaped owner on multi-profile pool: %v", gateway.accounts)
	}
	for _, accountID := range []string{"account-a", "account-b"} {
		wantScore := uint8(0)
		if accountID == "account-a" {
			wantScore = 1
		}
		if score := router.previousResponseFailureScore(accountID, "resp-dead", request.QuotaSelection, now); score != wantScore {
			t.Fatalf("negative cache %s = %d, want %d", accountID, score, wantScore)
		}
	}
	if owner, err := router.affinity.owner(t.Context(), affinityKeys{previous: "resp-dead"}, now); err != nil || owner != "account-a" {
		t.Fatalf("multi-owner fallback changed original binding: owner=%q err=%v", owner, err)
	}
}
