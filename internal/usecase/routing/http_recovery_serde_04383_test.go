package routing

import (
	"context"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Prodex 0.438.3 evaluates recovery from serde_json::Value. JSON with an
// invalid Unicode scalar, floating-number overflow, or a parser-depth
// violation is not a replayable full history even when all relevant fields
// (previous_response_id, session_id and input) would otherwise parse in Go.
func recoverySerdeFixtures04383() []struct {
	name, payload string
	valid         bool
} {
	prefix := `{"model":"gpt-6","session_id":"verified-session","previous_response_id":"resp-dead",` + fullHistoryPresenceInput04383 + `,"note":`
	invalidUTF8 := prefix + `"raw-` + string([]byte{0xff}) + `"}`
	deep := strings.Repeat(`{"child":`, 128) + `0` + strings.Repeat(`}`, 128)
	return []struct {
		name, payload string
		valid         bool
	}{
		{"valid-ascii", prefix + `"ok"}`, true},
		{"valid-max-u64", prefix + `18446744073709551615}`, true},
		{"valid-finite-exp", prefix + `1e308}`, true},
		{"valid-escaped-surrogate", prefix + `"\\ud800"}`, true},
		{"invalid-utf8", invalidUTF8, false},
		{"invalid-high-surrogate", prefix + `"\ud800"}`, false},
		{"invalid-low-surrogate", prefix + `"\udc00"}`, false},
		{"invalid-float-overflow", prefix + `1e309}`, false},
		{"invalid-float-negative-overflow", prefix + `-1e99999}`, false},
		{"invalid-deep-json", prefix + deep + `}`, false},
	}
}

func TestProdex04383FullHistoryRecoveryRejectsMalformedSerdeJSON(t *testing.T) {
	for _, tc := range recoverySerdeFixtures04383() {
		t.Run(tc.name, func(t *testing.T) {
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header), Body: []byte(tc.payload),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			recovered, ok := fullHistoryRecoveryRequest(request)
			if ok != tc.valid {
				t.Fatalf("malformed serde JSON unexpectedly recoverable=%v want=%v", ok, tc.valid)
			}
			wantPrevious := ""
			if tc.valid {
				wantPrevious = "resp-dead"
			}
			if got := requestPreviousResponseID(request.Body); got != wantPrevious {
				t.Fatalf("invalid JSON previous response=%q want=%q", got, wantPrevious)
			}
			if ok && requestPreviousResponseID(recovered.Body) != "" {
				t.Fatalf("recovered request retained previous response: %s", recovered.Body)
			}
			if string(request.Body) != tc.payload {
				t.Fatal("source request bytes were changed")
			}
		})
	}
}

func TestProdex04383MalformedHistoryDoesNotRetryOwnedPrevious(t *testing.T) {
	for _, tc := range recoverySerdeFixtures04383() {
		if tc.valid {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{
				{http.StatusBadRequest, "application/json", invalidPreviousBody04383},
				{http.StatusOK, "application/json", `{"object":"response","id":"must-not-reach"}`},
			}}
			router, err := NewRouter(Config{Gateway: gateway, PreferredAccount: "account-a", Accounts: func(context.Context) ([]proxymodel.Account, error) {
				return []proxymodel.Account{{ID: "account-a", Home: "/synthetic/account", Enabled: true}}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), "account-a", affinityKeys{previous: "resp-dead"}, router.now()); err != nil {
				t.Fatal(err)
			}
			exchange, err := router.Forward(t.Context(), proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
				Header: make(http.Header), Body: []byte(tc.payload), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}})
			if err != nil {
				t.Fatal(err)
			}
			defer exchange.Close()
			if len(gateway.requests) != 1 || string(gateway.requests[0].Body) != tc.payload {
				t.Fatalf("invalid serde JSON replayed or rewritten: attempts=%d", len(gateway.requests))
			}
		})
	}
}

func TestProdex04383MalformedTurnMetadataCannotAuthorizeHTTPReplay(t *testing.T) {
	requestBody := `{"model":"gpt-6","previous_response_id":"resp-dead",` + fullHistoryPresenceInput04383 + `}`
	for _, tc := range []struct {
		name, metadata string
		valid          bool
	}{
		{"valid-session", `{"session_id":"verified-session"}`, true},
		{"valid-nested", `{"client_metadata":{"session_id":"verified-session"}}`, true},
		{"valid-escaped-surrogate", `{"session_id":"verified-session","note":"\\ud800"}`, true},
		{"invalid-high-surrogate", `{"session_id":"verified-session","note":"\ud800"}`, false},
		{"invalid-low-surrogate", `{"session_id":"verified-session","note":"\udc00"}`, false},
		{"invalid-float", `{"session_id":"verified-session","unused":1e309}`, false},
		{"invalid-utf8", `{"session_id":"verified-session","note":"` + string([]byte{0xff}) + `"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses", Header: make(http.Header),
				Body: []byte(requestBody), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
			request.Header.Set("x-codex-turn-metadata", tc.metadata)
			if _, ok := fullHistoryRecoveryRequest(request); ok != tc.valid {
				t.Fatalf("invalid turn-metadata authorized replay: %v want=%v", ok, tc.valid)
			}
		})
	}
}
