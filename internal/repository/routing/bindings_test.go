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

func TestRoutingSnapshotRejectsNegativeBindingTimestamp(t *testing.T) {
	home := t.TempDir()
	content := `{"version":1,"bindings":[{"kind":"session","key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","account_id":"11111111111111111111111111111111","updated_unix":-1}]}`
	if err := os.WriteFile(filepath.Join(home, "routing.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(home).Load(context.Background()); err == nil {
		t.Fatal("negative binding timestamp accepted")
	}
}

func TestRoutingSnapshotRejectsPartialStateWithoutOverwritingIt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "routing.json")
	partial := `{"version":1,"bindings":[`
	if err := os.WriteFile(path, []byte(partial), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(root)
	if _, err := store.Load(context.Background()); err == nil {
		t.Fatal("partial routing snapshot was treated as empty state")
	}
	if _, err := store.Merge(context.Background(), nil); err == nil {
		t.Fatal("merge overwrote a partial routing snapshot")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != partial {
		t.Fatalf("partial routing snapshot changed to %q, error = %v", content, err)
	}
}

func TestRoutingSnapshotRestoresLastGoodAfterRestart(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Now().Unix()
	want := []routingentity.Binding{
		{Kind: "session", Key: strings.Repeat("a", 64), AccountID: strings.Repeat("1", 32), UpdatedUnix: now},
		{Kind: "previous", Key: strings.Repeat("b", 64), AccountID: strings.Repeat("2", 32), UpdatedUnix: now},
	}
	if _, err := store.Merge(context.Background(), want[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Merge(context.Background(), want[1:]); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(store.backupPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store.snapshotPath()); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(root).Load(context.Background())
	if err != nil || len(loaded) != 2 {
		t.Fatalf("missing primary recovery = %#v, %v", loaded, err)
	}
	if err := os.WriteFile(store.snapshotPath(), []byte(`{"version":1,"bindings":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err = NewStore(root).Load(context.Background())
	if err != nil || len(loaded) != 2 {
		t.Fatalf("partial primary recovery = %#v, %v", loaded, err)
	}
	repaired, err := os.ReadFile(store.snapshotPath())
	if err != nil || string(repaired) != string(backup) {
		t.Fatalf("repaired primary differs from last-good copy: %v", err)
	}
	loaded, err = NewStore(root).Load(context.Background())
	if err != nil || len(loaded) != 2 {
		t.Fatalf("repeated recovery load = %#v, %v", loaded, err)
	}
}

func TestRoutingSnapshotFailsClosedWhenBothCopiesAreCorrupt(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	if _, err := store.Merge(context.Background(), []routingentity.Binding{{
		Kind: "session", Key: strings.Repeat("a", 64), AccountID: strings.Repeat("1", 32), UpdatedUnix: time.Now().Unix(),
	}}); err != nil {
		t.Fatal(err)
	}
	primary, backup := []byte(`{"version":1,"bindings":[`), []byte("not-json")
	if err := os.WriteFile(store.snapshotPath(), primary, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.backupPath(), backup, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root).Load(context.Background()); err == nil {
		t.Fatal("corrupt routing snapshots were treated as empty state")
	}
	gotPrimary, primaryErr := os.ReadFile(store.snapshotPath())
	gotBackup, backupErr := os.ReadFile(store.backupPath())
	if primaryErr != nil || backupErr != nil || string(gotPrimary) != string(primary) || string(gotBackup) != string(backup) {
		t.Fatalf("corrupt snapshots changed: primary=%q/%v backup=%q/%v", gotPrimary, primaryErr, gotBackup, backupErr)
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

func TestRemoveRoutingBindingsPreservesUnrelatedOwners(t *testing.T) {
	store := NewStore(t.TempDir())
	first := routingentity.Binding{Kind: "session", Key: strings.Repeat("a", 64), AccountID: strings.Repeat("c", 32), UpdatedUnix: 1}
	second := routingentity.Binding{Kind: "thread", Key: strings.Repeat("b", 64), AccountID: strings.Repeat("d", 32), UpdatedUnix: 2}
	if _, err := store.Merge(context.Background(), []routingentity.Binding{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(context.Background(), []string{first.Key}); err != nil {
		t.Fatal(err)
	}
	values, err := store.Load(context.Background())
	if err != nil || len(values) != 1 || values[0].Key != second.Key {
		t.Fatalf("remaining routing bindings = %#v, error = %v", values, err)
	}
	if err := store.Remove(context.Background(), []string{"unsafe"}); err == nil {
		t.Fatal("invalid routing key accepted")
	}
}
