package routing

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04370EphemeralProviderResponseAffinityIsNotDurable(t *testing.T) {
	for _, fixture := range []struct {
		name          string
		ephemeral     bool
		wantPersisted bool
	}{
		{"transient_provider_key", true, false},
		{"managed_provider_profile", false, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			repository := routingrepo.NewStore(t.TempDir())
			const owner = "aabbccddeeff00112233445566778899"
			const responseID = "resp_deepseek_fixture"
			gateway := &responseRecoveryGateway{replies: []responseRecoveryReply{{
				status: http.StatusOK, contentType: "application/json",
				body: `{"object":"response","id":"resp_deepseek_fixture","output":[]}`,
			}}}
			router, err := NewRouter(Config{
				Gateway: gateway, Bindings: repository,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{
						ID: owner, Home: t.TempDir(), Enabled: true,
						Provider:        proxymodel.Provider{Kind: "deepseek"},
						EphemeralAPIKey: fixture.ephemeral,
					}}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			request := proxymodel.Request{Header: make(http.Header),
				QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses},
			}
			exchange, err := router.Forward(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if err := exchange.Close(); err != nil {
				t.Fatal(err)
			}
			memoryOwner, err := router.affinity.owner(t.Context(), affinityKeys{previous: responseID}, router.now())
			if err != nil || memoryOwner != owner {
				t.Fatalf("within-process previous response owner=%q, err=%v", memoryOwner, err)
			}
			bindings, err := repository.Load(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if got := len(bindings) > 0; got != fixture.wantPersisted {
				t.Fatalf("durable previous response bindings=%#v, wantPersisted=%t", bindings, fixture.wantPersisted)
			}
		})
	}
}

func TestProdex04370EphemeralWebSocketTurnStateIsNeverWrittenToProfileHome(t *testing.T) {
	const owner = "aabbccddeeff00112233445566778899"
	const previous = "resp_websocket_transient"
	const turnState = "turn-only-in-this-process"
	for _, fixture := range []struct {
		name      string
		ephemeral bool
		wantFile  bool
	}{
		{"transient", true, false},
		{"managed", false, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			profileHome := t.TempDir()
			if err := os.Chmod(profileHome, 0o700); err != nil {
				t.Fatal(err)
			}
			bindings := routingrepo.NewStore(t.TempDir())
			router, err := NewRouter(Config{
				Bindings: bindings, Gateway: &countingGateway{},
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{
						ID: owner, Home: profileHome, Enabled: true,
						EphemeralAPIKey: fixture.ephemeral,
					}}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := router.loadAccounts(t.Context()); err != nil {
				t.Fatal(err)
			}
			now := router.now()
			router.rememberAccountTurnState(t.Context(), previous, owner, profileHome, turnState, now)
			if got := router.affinity.responseTurnState(previous, owner, now); got != turnState {
				t.Fatalf("turn state not available within process: %q", got)
			}
			saved := filepath.Join(profileHome, ".godex-turn-state", affinityDigest("previous", previous)+".json")
			_, statErr := os.Stat(saved)
			if savedFile := statErr == nil; savedFile != fixture.wantFile {
				t.Fatalf("turn-state sidecar exists=%t want=%t err=%v", savedFile, fixture.wantFile, statErr)
			}
		})
	}
}
