package routing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04384RawKeyRejectsInvalidContinuationIdentity(t *testing.T) {
	for _, field := range []string{"previous", "session", "turn"} {
		for _, fixture := range []struct {
			name, value string
			reject      bool
		}{
			{"ascii_limit", strings.Repeat("x", 1024), false},
			{"ascii_over_limit", strings.Repeat("x", 1025), true},
			{"unicode_limit", strings.Repeat("雪", 341) + "x", false},
			{"unicode_over_limit", strings.Repeat("雪", 342), true},
			{"blank", " \t ", false},
			{"control_inside", "synthetic\x00context", true},
		} {
			t.Run(field+"/"+fixture.name, func(t *testing.T) {
				gateway := &countingGateway{}
				router, err := NewRouter(Config{Gateway: gateway,
					Accounts: func(context.Context) ([]proxymodel.Account, error) {
						return []proxymodel.Account{{ID: strings.Repeat("a", 32), Home: t.TempDir(),
							Enabled: true, EphemeralAPIKey: true, Provider: proxymodel.Provider{Kind: "deepseek"}}}, nil
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				request := proxymodel.Request{Header: make(http.Header),
					QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
				if field == "turn" {
					request.Header.Set("X-Codex-Turn-State", fixture.value)
				} else {
					name := "previous_response_id"
					if field == "session" {
						name = "session_id"
					}
					request.Body, err = json.Marshal(map[string]any{name: fixture.value})
					if err != nil {
						t.Fatal(err)
					}
				}
				exchange, err := router.Forward(t.Context(), request)
				if !fixture.reject {
					if err != nil || exchange == nil || len(gateway.owners) != 1 {
						t.Fatalf("valid identity was not dispatched once: err=%v calls=%d", err, len(gateway.owners))
					}
					if err := exchange.Close(); err != nil {
						t.Fatal(err)
					}
					return
				}
				if exchange != nil {
					_ = exchange.Close()
				}
				var routeError *proxymodel.Error
				if exchange != nil || len(gateway.owners) != 0 || !errors.As(err, &routeError) ||
					routeError.StatusCode != http.StatusServiceUnavailable || routeError.Message != "bound continuation provider is unavailable" {
					t.Fatalf("invalid identity rejection = %v calls=%d", err, len(gateway.owners))
				}
			})
		}
	}
}
