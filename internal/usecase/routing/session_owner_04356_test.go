package routing

import (
	"context"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
)

func TestProdex04356ForgetSessionRemovesDurableSessionAliasesIdempotently(t *testing.T) {
	store := routingrepo.NewStore(t.TempDir())
	const sessionID = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44fc"
	owner := strings.Repeat("1", 32)
	otherOwner := strings.Repeat("2", 32)
	now := time.Now().Unix()
	updates := []routingentity.Binding{
		{Kind: "thread", Key: affinityDigest("thread", sessionID), AccountID: owner, UpdatedUnix: now},
		{Kind: "session", Key: affinityDigest("session", sessionID), AccountID: owner, UpdatedUnix: now},
		{Kind: "session", Key: affinityDigest("session", "__compact_session__:"+sessionID), AccountID: owner, UpdatedUnix: now},
		{Kind: "previous", Key: affinityDigest("previous", "resp-unrelated"), AccountID: otherOwner, UpdatedUnix: now},
	}
	if _, err := store.Merge(context.Background(), updates); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := ForgetSession(context.Background(), store, sessionID); err != nil {
			t.Fatal(err)
		}
	}
	values, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Kind != "previous" || values[0].AccountID != otherOwner {
		t.Fatalf("durable bindings after delete = %#v", values)
	}
}
