package routing

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// The tagged semantic planner can only classify full-history recoverability
// after parsing a complete serde_json::Value. Invalid unrelated JSON must
// never authorize a WebSocket/Responses precommit fallback.
func TestProdex04383MalformedJSONCannotClaimReconstructableTurnHistory(t *testing.T) {
	for _, tc := range recoverySerdeFixtures04383() {
		t.Run(tc.name, func(t *testing.T) {
			request := proxymodel.Request{Header: make(http.Header), Body: []byte(tc.payload)}
			if got := requestHasReconstructableFullHistory(request); got != tc.valid {
				t.Fatalf("invalid whole JSON classified as replayable=%t want=%t", got, tc.valid)
			}
		})
	}
}

// The recovery scrub may remove obsolete turn headers, but may not re-encode
// an invalid JSON message. Go RawMessage otherwise replaces unpaired Unicode
// with U+FFFD and would silently rewrite an invalid original input.
func TestProdex04383TurnScrubKeepsMalformedRequestJSONOpaque(t *testing.T) {
	for _, tc := range recoverySerdeFixtures04383() {
		if tc.valid {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			// Preserve the original invalid note (byte/surrogate/number/depth)
			// and include both direct and nested obsolete turn-state members.
			body := strings.Replace(tc.payload, `"previous_response_id":"resp-dead",`,
				`"previous_response_id":"resp-dead","x-codex-turn-state":"dead-turn","client_metadata":{"x-codex-turn-state":"dead-turn"},`, 1)
			request := proxymodel.Request{Header: http.Header{"X-Codex-Turn-State": {"dead-turn"}}, Body: []byte(body)}
			clean := requestWithoutTurnState(request)
			if !bytes.Equal(clean.Body, request.Body) {
				t.Fatal("malformed request was reserialized into a different body")
			}
			if clean.Header.Get("x-codex-turn-state") != "" {
				t.Fatalf("obsolete turn header not cleared: %+v", clean.Header)
			}
			if request.Header.Get("x-codex-turn-state") != "dead-turn" {
				t.Fatal("original caller header mutated")
			}
		})
	}
}

// A precommit quota failure of an owned WebSocket turn may release that
// turn and replay a reconstructable transcript on a separate Ready account.
// The source only permits that recovery for valid complete JSON, regardless
// of a valid turn-state header on the malformed message.
func TestProdex04383MalformedWebSocketHistoryCannotRecoverOnAlternate(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		valid      bool
	}{
		{"invalid-surrogate", `"\ud800"`, false},
		{"invalid-float", `1e309`, false},
		{"valid-surrogate", `"\ud83d\ude00"`, true},
		{"valid-finite", `1e308`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &websocketDispatchGateway{responses: []*proxymodel.Response{
				websocketQuotaFailureResponse(), websocketCommittedTestResponse("resp-replayed"),
			}}
			router, err := NewRouter(Config{Gateway: gateway, PreferredAccount: websocketQuotaOwnerA,
				Accounts: func(context.Context) ([]proxymodel.Account, error) { return websocketQuotaAccounts(), nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err := router.affinity.remember(t.Context(), websocketQuotaOwnerA, affinityKeys{turn: "turn-owned"}, router.now()); err != nil {
				t.Fatal(err)
			}
			body := `{"type":"response.create","session_id":"session-valid","input":[{"role":"user","content":"first"},{"role":"assistant","content":"reply"},{"role":"user","content":"next"}],"note":` + tc.note + `}`
			request := websocketDispatchRequest(body, 843399)
			request.Header.Set("x-codex-turn-state", "turn-owned")
			result, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Close()
			wantCalls := 1
			if tc.valid {
				wantCalls = 2
			}
			if len(gateway.requests) != wantCalls {
				t.Fatalf("WebSocket malformed turn recovery sent %d attempts want %d on %v", len(gateway.requests), wantCalls, gateway.accounts)
			}
			if string(gateway.requests[0].Body) != body {
				t.Fatal("first invalid JSON message was altered")
			}
		})
	}
}
