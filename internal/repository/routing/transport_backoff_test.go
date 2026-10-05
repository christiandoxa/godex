package routing

import (
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestProdex04355TransportBackoffStartupSoftensAndClearsByRoute(t *testing.T) {
	now := time.Unix(80_000, 0)
	store := NewStore(t.TempDir())
	account := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, item := range []routingentity.TransportBackoff{
		{AccountID: account, Route: "responses", UntilUnix: now.Unix() + 90},
		{AccountID: account, Route: "compact", UntilUnix: now.Unix() + 30},
	} {
		if err := store.SetTransportBackoff(t.Context(), item, now); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := NewStore(store.root).LoadTransportBackoffs(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("startup backoffs = %+v", loaded)
	}
	for _, backoff := range loaded {
		if backoff.UntilUnix != now.Unix()+15 {
			t.Fatalf("softened backoff = %+v, want +15 seconds", backoff)
		}
	}
	if err := store.ClearTransportBackoff(t.Context(), account, "responses"); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadTransportBackoffs(t.Context(), now)
	if err != nil || len(loaded) != 1 || loaded[0].Route != "compact" {
		t.Fatalf("route clear = %+v err=%v", loaded, err)
	}
}
