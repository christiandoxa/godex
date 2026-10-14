package routing

import (
	"context"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex043711ExplicitAbsentSessionReleaseFencesStaleWorker(t *testing.T) {
	store := routingrepo.NewStore(t.TempDir())
	const session = "sess-released-before-writer"
	const ownerOld = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const ownerNew = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	oldTime := time.Now().Add(-time.Hour).Unix()
	keys := []string{session, "__compact_session__:" + session}
	stale := make([]routingentity.Binding, 0, len(keys))
	for _, value := range keys {
		stale = append(stale, routingentity.Binding{
			Kind: "session", Key: affinityDigest("session", value),
			AccountID: ownerOld, UpdatedUnix: oldTime,
		})
	}
	router, err := NewRouter(Config{
		Bindings: store, Gateway: &countingGateway{},
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: ownerNew, Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	// The owner was already removed by a previous incomplete migration.
	// A repeated explicit release must nevertheless publish a barrier.
	if err := router.ReleaseSessionAffinity(t.Context(), session); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Merge(t.Context(), stale); err != nil {
		t.Fatalf("stale writer merge: %v", err)
	}
	loaded, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("absent release allowed old owner resurrection: %+v", loaded)
	}
	for _, value := range stale {
		value.AccountID = ownerNew
		value.UpdatedUnix = oldTime
		if _, err := store.MergeVerified(t.Context(), []routingentity.Binding{value}); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err = store.Load(t.Context())
	if err != nil || len(loaded) != 2 {
		t.Fatalf("subsequent rebind lost: %+v err=%v", loaded, err)
	}
	for _, binding := range loaded {
		if binding.AccountID != ownerNew {
			t.Fatalf("stale session owner remained: %+v", binding)
		}
	}
	if owner, err := SessionOwner(t.Context(), store, session); err != nil || owner != ownerNew {
		t.Fatalf("rebound owner=%q err=%v", owner, err)
	}
}
