package routing

import (
	"context"
	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProdex043711FullHistoryRepairsOnlyConflictingSoftSession(t *testing.T) {
	const session = "sess-conflicted"
	const ownerA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const ownerB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	fullHistory := `{"model":"gpt-5.6","session_id":"sess-conflicted","input":[{"role":"user","content":"first"},{"role":"assistant","content":"reply"},{"role":"user","content":"retry full history"}]}`
	for _, tc := range []struct {
		name, body string
		route      quotamodel.RouteKind
		wantOK     bool
	}{
		{"full history repairs soft session", fullHistory, quotamodel.RouteKindResponses, true},
		{"single turn cannot reset", `{"session_id":"sess-conflicted","input":[{"role":"user","content":"continue"}]}`, quotamodel.RouteKindResponses, false},
		{"opaque previous cannot reset", `{"previous_response_id":"resp-hard","session_id":"sess-conflicted","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}]}`, quotamodel.RouteKindResponses, false},
		{"opaque turn cannot reset", `{"x-codex-turn-state":"turn-hard","session_id":"sess-conflicted","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}]}`, quotamodel.RouteKindResponses, false},
		{"compact session must not reset", fullHistory, quotamodel.RouteKindCompact, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := routingrepo.NewStore(t.TempDir())
			seed := routingentity.Binding{Kind: "session", Key: affinityDigest("session", session), AccountID: ownerA, UpdatedUnix: 100}
			if _, err := repo.Merge(t.Context(), []routingentity.Binding{seed}); err != nil {
				t.Fatal(err)
			}
			seed.AccountID = ownerB
			seed.UpdatedUnix = 120
			if _, err := repo.MergeVerified(t.Context(), []routingentity.Binding{seed}); err != nil {
				t.Fatal(err)
			}
			gateway := &prodex04356Gateway{steps: []prodex04356Step{
				{status: http.StatusOK, body: `{"id":"resp-full-history"}`},
			}}
			router, err := NewRouter(Config{
				Gateway: gateway, Bindings: repo, PreferredAccount: ownerA,
				Now: func() time.Time { return time.Unix(1700000000, 0) },
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{
						{ID: ownerA, Home: "/synthetic/a", Enabled: true, RouteOrder: 1},
						{ID: ownerB, Home: "/synthetic/b", Enabled: true, RouteOrder: 2},
					}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			request := proxymodel.Request{Method: http.MethodPost, Path: "/backend-api/codex/responses",
				Header: make(http.Header), Body: []byte(tc.body),
				QuotaSelection: quotamodel.Selection{RouteKind: tc.route},
			}
			if strings.HasPrefix(tc.name, "opaque") {
				if err := router.repairConflictedFullHistorySession043711(t.Context(), request, requestRoutingAffinity(request)); err != nil {
					t.Fatal(err)
				}
				owner, err := SessionOwner(t.Context(), repo, session)
				if err != nil || owner != routingentity.ConflictAccountID {
					t.Fatalf("opaque request triggered full-history reset: owner=%q err=%v", owner, err)
				}
			}
			exchange, err := router.Forward(t.Context(), request)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("valid full-history reset failed: %v", err)
				}
				defer exchange.Close()
				if exchange.Result.Failed || exchange.Result.AccountID != ownerA || len(gateway.calls) != 1 {
					t.Fatalf("reset output owner=%q failed=%v upstream=%v", exchange.Result.AccountID, exchange.Result.Failed, gateway.calls)
				}
				if _, err := repo.Merge(t.Context(), []routingentity.Binding{{
					Kind: "session", Key: affinityDigest("session", session),
					AccountID: ownerB, UpdatedUnix: 100,
				}}); err != nil {
					t.Fatalf("stale worker poisoned repaired owner: %v", err)
				}
				owner, err := SessionOwner(t.Context(), repo, session)
				if err != nil || owner != ownerA {
					t.Fatalf("repaired session owner not durably rebound: owner=%q err=%v", owner, err)
				}
			} else {
				if exchange != nil {
					defer exchange.Close()
				}
				if strings.HasPrefix(tc.name, "opaque") {
					// Opaque request handling may independently recover a dead
					// turn token, but the full-history conflict reset is forbidden.
				} else {
					if err == nil || !strings.Contains(err.Error(), "conflicting conversation affinity") {
						t.Fatalf("unsafe reset accepted: err=%v upstream=%v", err, gateway.calls)
					}
					if len(gateway.calls) != 0 {
						t.Fatalf("unsafe request reached provider: %v", gateway.calls)
					}
				}
			}
		})
	}
}

func TestProdex043711FullHistoryRejectsOpaqueTurnStateInBodyMetadataAndHeader(t *testing.T) {
	body := []byte(`{"session_id":"sess-conflicted","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}],"client_metadata":{"x-codex-turn-state":"turn-meta"}}`)
	for _, tc := range []struct {
		name   string
		body   []byte
		header http.Header
	}{
		{"nested client_metadata", body, make(http.Header)},
		{"top-level", []byte(`{"session_id":"sess-conflicted","x-codex-turn-state":"turn-top","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}]}`), make(http.Header)},
		{"header", []byte(`{"session_id":"sess-conflicted","input":[{"role":"user"},{"role":"assistant"},{"role":"user"}]}`), http.Header{"X-Codex-Turn-State": []string{"turn-head"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := proxymodel.Request{
				Method: http.MethodPost, Header: tc.header, Body: tc.body,
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			}
			keys := requestRoutingAffinity(request)
			if keys.turn == "" {
				t.Fatal("opaque turn token was not detected")
			}
			if keys.hasSoftSessionAffinity(request.QuotaSelection) {
				t.Fatal("opaque turn token misclassified as repairable soft session")
			}
		})
	}
}
