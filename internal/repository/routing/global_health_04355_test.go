package routing

import (
	"testing"
	"time"
)

func TestProdex04355RoutingHealthPersistsGlobalProfileScore(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Unix(130_000, 0)
	if _, err := store.SetRouteHealth(t.Context(), "account-a", "global", 3, now); err != nil {
		t.Fatal(err)
	}
	scores, err := store.LoadRouteHealth(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(scores) != 1 || scores[0].AccountID != "account-a" || scores[0].Route != "global" || scores[0].Score != 3 {
		t.Fatalf("global health scores = %#v", scores)
	}
}
