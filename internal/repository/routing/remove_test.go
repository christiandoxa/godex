package routing

import (
	"context"
	"strings"
	"testing"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestRemovePreviousRoutingBindingPreservesUnrelatedOwners(t *testing.T) {
	store := NewStore(t.TempDir())
	ownerA := strings.Repeat("1", 32)
	ownerB := strings.Repeat("2", 32)
	first := routingentity.Binding{Kind: "previous", Key: strings.Repeat("a", 64), AccountID: ownerA, UpdatedUnix: 1}
	second := routingentity.Binding{Kind: "session", Key: strings.Repeat("b", 64), AccountID: ownerB, UpdatedUnix: 2}
	if _, err := store.Merge(context.Background(), []routingentity.Binding{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(context.Background(), []string{first.Key}); err != nil {
		t.Fatal(err)
	}
	values, err := store.Load(context.Background())
	if err != nil || len(values) != 1 || values[0].Key != second.Key {
		t.Fatalf("remaining=%#v err=%v", values, err)
	}
	if err := store.Remove(context.Background(), []string{"unsafe"}); err == nil {
		t.Fatal("invalid routing key accepted")
	}
}
