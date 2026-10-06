package routing

import (
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestProdex04355VerifiedBindingConflictPersistsAcrossReload(t *testing.T) {
	store := NewStore(t.TempDir())
	key := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	a := "0123456789abcdef0123456789abcdef"
	b := "fedcba9876543210fedcba9876543210"
	now := time.Now().Unix()
	if _, err := store.Merge(t.Context(), []routingentity.Binding{{Kind: "previous", Key: key, AccountID: a, UpdatedUnix: now}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeVerified(t.Context(), []routingentity.Binding{{Kind: "previous", Key: key, AccountID: b, UpdatedUnix: now + 1}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(store.root).Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].AccountID != routingentity.ConflictAccountID || loaded[0].UpdatedUnix != now+1 {
		t.Fatalf("verified conflict reload = %#v", loaded)
	}
	if _, err := store.MergeVerified(t.Context(), []routingentity.Binding{{Kind: "previous", Key: key, AccountID: a, UpdatedUnix: now + 2}}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].AccountID != routingentity.ConflictAccountID || loaded[0].UpdatedUnix != now+2 {
		t.Fatalf("conflict sentinel was overwritten = %#v", loaded)
	}
}
