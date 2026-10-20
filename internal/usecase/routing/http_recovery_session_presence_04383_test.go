package routing

import (
	"context"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const fullHistoryPresenceInput04383 = `"input":[{"role":"user","content":"first"},{"role":"assistant","content":"reply"},{"role":"user","content":"continue"}]`
const invalidPreviousBody04383 = `{"type":"error","status":400,"error":{"type":"invalid_request_error","message":"Invalid ` + "`previous_response_id`" + `."}}`

// The tagged 0.438.3 request semantic planner selects a direct JSON
// session_id by MEMBER PRESENCE. Null, whitespace, or an invalid direct
// member shadows nested client_metadata.session_id on a full-history retry.
func TestProdex04383FullHistoryRecoverySessionDirectPresenceShadowsNested(t *testing.T) {
	for _, tc := range []struct {
		name, fields, metadata string
		want                   bool
	}{
		{"nested-control", `"client_metadata":{"session_id":"verified-session"},`, "", true},
		{"direct-control", `"session_id":"verified-session",`, "", true},
		{"null-shadow", `"session_id":null,"client_metadata":{"session_id":"verified-session"},`, "", false},
		{"blank-shadow", `"session_id":"   ","client_metadata":{"session_id":"verified-session"},`, "", false},
		{"boolean-shadow", `"session_id":false,"client_metadata":{"session_id":"verified-session"},`, "", false},
		{"object-shadow", `"session_id":{},"client_metadata":{"session_id":"verified-session"},`, "", false},
		{"header-direct", `"session_id":null,`, `{"session_id":"verified-session"}`, true},
		{"header-nested", `"session_id":null,`, `{"client_metadata":{"session_id":"verified-session"}}`, true},
		{"header-null-shadow", ``, `{"session_id":null,"client_metadata":{"session_id":"verified-session"}}`, false},
		{"header-blank-shadow", ``, `{"session_id":" ","client_metadata":{"session_id":"verified-session"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"gpt-6","previous_response_id":"resp-dead",` + tc.fields + fullHistoryPresenceInput04383 + `}`
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			if tc.metadata != "" {
				request.Header.Set("x-codex-turn-metadata", tc.metadata)
			}
			result, canRetry := fullHistoryRecoveryRequest(request)
			if canRetry != tc.want {
				t.Fatalf("full-history session presence %s: retry=%v want=%v", tc.name, canRetry, tc.want)
			}
			if canRetry && requestPreviousResponseID(result.Body) != "" {
				t.Fatalf("full-history retry retained old previous response: %s", result.Body)
			}
			if string(request.Body) != body {
				t.Fatal("recovery predicate changed original request body")
			}
		})
	}
}

// Observe real transport retry counts after an owned stale HTTP response.
// The storage-mode invariant is handled by the session-identity unit tests;
// this gateway makes the extra replay observable independently.
func TestProdex04383HTTPInvalidPreviousRetryCannotBorrowNestedSession(t *testing.T) {
	for _, tc := range []struct {
		name, session string
		wantAttempts  int
	}{
		{"null", `"session_id":null,"client_metadata":{"session_id":"verified-session"},`, 1},
		{"blank", `"session_id":"  ","client_metadata":{"session_id":"verified-session"},`, 1},
		{"nested-control", `"client_metadata":{"session_id":"verified-session"},`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusBadRequest, "application/json", invalidPreviousBody04383},
				{http.StatusOK, "application/json", `{"object":"response","id":"resp-recovered"}`},
			}}
			config := Config{Gateway: gateway, PreferredAccount: "account-a", Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/profile", Enabled: true}}, nil
			}}
			router, err := NewRouter(config)
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{previous: "resp-dead"}, router.now()); err != nil {
				t.Fatal(err)
			}
			body := `{"model":"gpt-6","previous_response_id":"resp-dead",` + tc.session + fullHistoryPresenceInput04383 + `}`
			result, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
				Header: make(http.Header), Body: []byte(body), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}})
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			if len(gateway.requests) != tc.wantAttempts {
				t.Fatalf("invalid previous incorrectly retried: attempts=%d want=%d", len(gateway.requests), tc.wantAttempts)
			}
			if tc.wantAttempts == 1 && (result.Result.Response.StatusCode != http.StatusBadRequest || string(gateway.requests[0].Body) != body) {
				t.Fatalf("non-replayable original request changed: status=%d body=%s", result.Result.Response.StatusCode, gateway.requests[0].Body)
			}
			if tc.wantAttempts == 2 && requestPreviousResponseID(gateway.requests[1].Body) != "" {
				t.Fatal("recovered request still has invalid previous response")
			}
		})
	}
}
