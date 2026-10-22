package routing

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04384RawKeyLocalRoutesDoNotConsumeOrCreateBindings(t *testing.T) {
	for _, provider := range []string{"deepseek", "gemini"} {
		for _, path := range []string{"/responses/compact", "/models", "/models/model-fixture"} {
			t.Run(provider+path, func(t *testing.T) {
				repository := routingrepo.NewStore(t.TempDir())
				profileHome := t.TempDir()
				gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{{status: 200,
					contentType: "application/json", body: `{"id":"local-response","output":[]}`}}}
				router, err := NewRouter(Config{Gateway: gateway, Bindings: repository,
					Accounts: func(context.Context) ([]proxymodel.Account, error) {
						return []proxymodel.Account{{ID: "aabbccddeeff00112233445566778899", Home: profileHome, Enabled: true,
							EphemeralAPIKey: true, Provider: proxymodel.Provider{Kind: provider}}}, nil
					}})
				if err != nil {
					t.Fatal(err)
				}
				if err := router.affinity.rememberVerified(t.Context(), routingentity.ConflictAccountID,
					affinityKeys{previous: "bound-response"}, router.now()); err != nil {
					t.Fatal(err)
				}
				before, err := repository.Load(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				request := proxymodel.Request{Method: http.MethodGet, Path: path,
					Header: http.Header{"Session-Id": {"local-session"}},
					Body:   []byte(`{"previous_response_id":"bound-response"}`)}
				if path == "/responses/compact" {
					request.Method = http.MethodPost
					request.QuotaSelection.RouteKind = quotamodel.RouteKindCompact
				}
				exchange, err := router.Forward(t.Context(), request)
				if err != nil {
					t.Fatalf("local route consumed the response binding: %v", err)
				}
				if err := exchange.Close(); err != nil {
					t.Fatal(err)
				}
				after, err := repository.Load(t.Context())
				if err != nil || !reflect.DeepEqual(before, after) || len(gateway.requests) != 1 {
					t.Fatalf("local route changed durable bindings or dispatch count: %v", err)
				}
				owner, err := router.affinity.owner(t.Context(), affinityKeys{previous: "local-response", session: "local-session"}, router.now())
				if err != nil || owner != "" {
					t.Fatalf("local route created a continuation owner: %q %v", owner, err)
				}
			})
		}
	}
}
