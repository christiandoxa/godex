package routing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestBindingSnapshotMergesConcurrentOwnersAndRejectsConflicts(t *testing.T) {
	home := t.TempDir()
	store := NewStore(home)
	now := time.Now().Unix()
	first := routingentity.Binding{Kind: "thread", Key: strings.Repeat("a", 64), AccountID: strings.Repeat("a", 32), UpdatedUnix: now}
	second := routingentity.Binding{Kind: "previous", Key: strings.Repeat("b", 64), AccountID: strings.Repeat("b", 32), UpdatedUnix: now}
	var group sync.WaitGroup
	for _, binding := range []routingentity.Binding{first, second} {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := NewStore(home).Merge(context.Background(), []routingentity.Binding{binding}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	values, err := store.Load(context.Background())
	if err != nil || len(values) != 2 {
		t.Fatalf("merged owners = %d, %v", len(values), err)
	}
	first.AccountID = second.AccountID
	if _, err := store.Merge(context.Background(), []routingentity.Binding{first}); err == nil {
		t.Fatal("owner conflict accepted")
	}
}
func TestRoutingSnapshotRejectsUnknownVersionsAndUnsafeFiles(t *testing.T) {
	for _, content := range []string{`{"version":2,"bindings":[]}`, `{"version":1,"bindings":[]} {}`, `{"version":1,"bindings":[{"key":"unsafe","account_id":"unsafe"}]}`} {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "routing.json"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(home).Load(context.Background()); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}
func TestThreadOwnershipDoesNotExpire(t *testing.T) {
	store := NewStore(t.TempDir())
	binding := routingentity.Binding{Kind: "thread", Key: strings.Repeat("a", 64), AccountID: strings.Repeat("a", 32), UpdatedUnix: 1}
	if _, err := store.Merge(context.Background(), []routingentity.Binding{binding}); err != nil {
		t.Fatal(err)
	}
	values, err := store.Load(context.Background())
	if err != nil || len(values) != 1 {
		t.Fatalf("forgotten thread: %v", err)
	}
}
