package routing

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
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

// Older Godex builds durably recorded synthetic previous_response bindings.
// A new transient-key launch must ignore those entries, without deleting
// persistent managed-profile affinity or overwriting the user's state.
func TestProdex04370LegacyEphemeralAffinityCannotHijackNewLaunch(t *testing.T) {
	const transientID = "aabbccddeeff00112233445566778899"
	const managedID = "11223344556677889900aabbccddeeff"
	repo := routingrepo.NewStore(t.TempDir())
	now := time.Now()
	oldKey := affinityKeys{previous: "resp-old-transient"}.entries()[0]
	oldKey.AccountID = transientID
	oldKey.UpdatedUnix = now.Unix()
	managedKey := affinityKeys{previous: "resp-managed"}.entries()[0]
	managedKey.AccountID = managedID
	managedKey.UpdatedUnix = now.Unix()
	var oldEntries = []routingentity.Binding{oldKey, managedKey}
	if _, err := repo.Merge(t.Context(), oldEntries); err != nil {
		t.Fatal(err)
	}
	source := func(context.Context) ([]proxymodel.Account, error) {
		return []proxymodel.Account{
			{ID: transientID, Enabled: true, EphemeralAPIKey: true},
			{ID: managedID, Enabled: true},
		}, nil
	}
	newRouter := func() *Router {
		r, err := NewRouter(Config{Bindings: repo, Gateway: &countingGateway{}, Accounts: source, Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := newRouter()
	// Simulate a stale cached binding before source/account registration.
	staleOwner, err := first.affinity.owner(t.Context(), affinityKeys{previous: "resp-old-transient"}, now)
	if err != nil || staleOwner != transientID {
		t.Fatalf("fixture old owner=%q err=%v", staleOwner, err)
	}
	if _, err := first.loadAccounts(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ key, want string }{
		{"resp-old-transient", ""},
		{"resp-managed", managedID},
	} {
		owner, err := first.affinity.owner(t.Context(), affinityKeys{previous: fixture.key}, now)
		if err != nil || owner != fixture.want {
			t.Fatalf("post-upgrade %q owner=%q want=%q err=%v", fixture.key, owner, fixture.want, err)
		}
	}
	if err := first.rememberVerifiedAccountBinding(t.Context(), transientID, affinityKeys{previous: "resp-new-transient"}, now); err != nil {
		t.Fatal(err)
	}
	owner, err := first.affinity.owner(t.Context(), affinityKeys{previous: "resp-new-transient"}, now)
	if err != nil || owner != transientID {
		t.Fatalf("current transient owner=%q err=%v", owner, err)
	}
	second := newRouter()
	if _, err := second.loadAccounts(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ key, want string }{
		{"resp-old-transient", ""},
		{"resp-new-transient", ""},
		{"resp-managed", managedID},
	} {
		owner, err := second.affinity.owner(t.Context(), affinityKeys{previous: fixture.key}, now)
		if err != nil || owner != fixture.want {
			t.Fatalf("restart %q owner=%q want=%q err=%v", fixture.key, owner, fixture.want, err)
		}
	}
	persisted, err := repo.Load(t.Context())
	if err != nil || len(persisted) != 2 {
		t.Fatalf("upgrade must be non-destructive: persisted=%v err=%v", persisted, err)
	}
}
