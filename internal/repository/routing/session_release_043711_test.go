package routing

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

// Tagged 0.437.11 keeps a monotonic session release barrier independent
// of the transient verified/dead lifecycle state, across stale worker
// snapshots, persistent reload and same-clock-second rebinding.
func TestProdex043711ReleasedSessionCannotBeResurrectedByStaleSnapshot(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewStore(root)
	ownerA, ownerB := strings.Repeat("a", 32), strings.Repeat("b", 32)
	sessionKey, responseKey := strings.Repeat("1", 64), strings.Repeat("2", 64)
	oldSession := routingentity.Binding{Kind: "session", Key: sessionKey, AccountID: ownerA, UpdatedUnix: 100}
	response := routingentity.Binding{Kind: "previous", Key: responseKey, AccountID: ownerA, UpdatedUnix: time.Now().Unix()}
	if _, err := store.Merge(ctx, []routingentity.Binding{oldSession, response}); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, []string{sessionKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root).Merge(ctx, []routingentity.Binding{oldSession}); err != nil {
		t.Fatalf("old worker session snapshot caused conflict: %v", err)
	}
	now := time.Now().Unix()
	next := routingentity.Binding{Kind: "session", Key: sessionKey, AccountID: ownerB, UpdatedUnix: now}
	if _, err := NewStore(root).MergeVerified(ctx, []routingentity.Binding{next}); err != nil {
		t.Fatalf("valid verified session rebind after release: %v", err)
	}
	replay := NewStore(root)
	if _, err := replay.Merge(ctx, []routingentity.Binding{oldSession}); err != nil {
		t.Fatalf("stale snapshot poisoned verified owner after restart: %v", err)
	}
	values, err := replay.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owners := map[string]routingentity.Binding{}
	for _, item := range values {
		owners[item.Key] = item
	}
	if got := owners[sessionKey]; got.AccountID != ownerB || got.UpdatedUnix <= oldSession.UpdatedUnix {
		t.Fatalf("released session owner regressed: %+v", got)
	}
	if got := owners[responseKey]; got.AccountID != ownerA {
		t.Fatalf("immutable previous response changed after session release: %+v", got)
	}
}

func TestProdex043711SameSecondReleaseRebindAdvancesEpoch(t *testing.T) {
	store := NewStore(t.TempDir())
	ctx := t.Context()
	key := strings.Repeat("3", 64)
	a := routingentity.Binding{Kind: "session", Key: key, AccountID: strings.Repeat("a", 32), UpdatedUnix: 100}
	if _, err := store.Merge(ctx, []routingentity.Binding{a}); err != nil {
		t.Fatal(err)
	}
	var previous int64 = 100
	for iteration := 0; iteration < 4; iteration++ {
		stale, err := store.Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Remove(ctx, []string{key}); err != nil {
			t.Fatal(err)
		}
		owner := strings.Repeat("a", 32)
		if iteration%2 == 0 {
			owner = strings.Repeat("b", 32)
		}
		rebound := routingentity.Binding{Kind: "session", Key: key, AccountID: owner, UpdatedUnix: 100}
		if _, err := store.MergeVerified(ctx, []routingentity.Binding{rebound}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Merge(ctx, stale); err != nil {
			t.Fatalf("iteration %d stale worker conflict: %v", iteration, err)
		}
		loaded, err := NewStore(store.root).Load(ctx)
		if err != nil || len(loaded) != 1 || loaded[0].AccountID != owner ||
			loaded[0].UpdatedUnix <= previous {
			t.Fatalf("iteration %d owner/epoch = %#v err=%v previous=%d", iteration, loaded, err, previous)
		}
		previous = loaded[0].UpdatedUnix
	}
}

func TestProdex043711UnreleasedSessionConflictStillImmutable(t *testing.T) {
	ctx := t.Context()
	store := NewStore(t.TempDir())
	key := strings.Repeat("4", 64)
	first := routingentity.Binding{Kind: "session", Key: key, AccountID: strings.Repeat("a", 32), UpdatedUnix: 100}
	second := routingentity.Binding{Kind: "session", Key: key, AccountID: strings.Repeat("b", 32), UpdatedUnix: 120}
	if _, err := store.Merge(ctx, []routingentity.Binding{first}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeVerified(ctx, []routingentity.Binding{second}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx)
	if err != nil || len(loaded) != 1 || loaded[0].AccountID != routingentity.ConflictAccountID {
		t.Fatalf("genuine unreleased conflict was erased: %#v %v", loaded, err)
	}
}

func TestProdex043711SessionReleaseFenceSurvivesBackupRecovery(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	repo := NewStore(root)
	key := strings.Repeat("5", 64)
	a := routingentity.Binding{Kind: "session", Key: key, AccountID: strings.Repeat("a", 32), UpdatedUnix: 100}
	b := routingentity.Binding{Kind: "session", Key: key, AccountID: strings.Repeat("b", 32), UpdatedUnix: 101}
	if _, err := repo.Merge(ctx, []routingentity.Binding{a}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Remove(ctx, []string{key}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MergeVerified(ctx, []routingentity.Binding{b}); err != nil {
		t.Fatal(err)
	}
	// A stale worker's snapshot and a crash-corrupted primary must not
	// erase the release barrier persisted in the validated backup.
	if err := os.WriteFile(repo.snapshotPath(), []byte(`{"version":1,"bindings":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	recovered := NewStore(root)
	if _, err := recovered.Merge(ctx, []routingentity.Binding{a}); err != nil {
		t.Fatalf("backup recovery resurrected stale account: %v", err)
	}
	items, err := recovered.Load(ctx)
	if err != nil || len(items) != 1 || items[0].AccountID != b.AccountID {
		t.Fatalf("backup recovery owner=%#v err=%v", items, err)
	}
	// The new field is optional, preserving legacy version-1 stores.
	legacy := NewStore(t.TempDir())
	if _, err := legacy.Merge(ctx, []routingentity.Binding{a}); err != nil {
		t.Fatal(err)
	}
	if items, err := legacy.Load(ctx); err != nil || len(items) != 1 {
		t.Fatalf("legacy routing store cannot be read: %#v %v", items, err)
	}
}

func TestProdex043711SessionReleaseRejectsMalformedEpochs(t *testing.T) {
	root := t.TempDir()
	testCases := []string{
		`{"version":1,"bindings":[],"session_release_epochs":{"invalid":100}}`,
		`{"version":1,"bindings":[],"session_release_epochs":{"` + strings.Repeat("a", 64) + `":-1}}`,
		`{"version":1,"bindings":[],"session_release_epochs":{"` + strings.Repeat("g", 64) + `":100}}`,
	}
	for _, body := range testCases {
		_ = os.Remove(filepath.Join(root, "routing.json.last-good"))
		if err := os.WriteFile(filepath.Join(root, "routing.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewStore(root).Load(t.Context()); err == nil {
			t.Fatalf("accepted forged release epoch %s", body)
		}
	}
}

func TestProdex043711ReleasedSessionStillValidatesStaleInput(t *testing.T) {
	store := NewStore(t.TempDir())
	key := strings.Repeat("6", 64)
	old := routingentity.Binding{Kind: "session", Key: key, AccountID: strings.Repeat("a", 32), UpdatedUnix: 100}
	if _, err := store.Merge(t.Context(), []routingentity.Binding{old}); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(t.Context(), []string{key}); err != nil {
		t.Fatal(err)
	}
	invalid := old
	invalid.UpdatedUnix = -1
	if _, err := store.Merge(t.Context(), []routingentity.Binding{invalid}); err == nil {
		t.Fatal("stale invalid session input silently accepted")
	}
	if _, err := store.MergeVerified(t.Context(), []routingentity.Binding{invalid}); err == nil {
		t.Fatal("verified invalid timestamp was promoted past fence")
	}
}
