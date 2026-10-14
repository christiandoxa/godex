package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func releaseFenceKey043711(index int) string {
	return fmt.Sprintf("%064x", index)
}

// In tagged Prodex 0.437.11 retired session fences last seven days.
// An old retired fence is not permanent quota or an indefinite block.
func TestProdex043711ExpiredRetiredFencesCompactOnPersistentMerge(t *testing.T) {
	store := NewStore(t.TempDir())
	old := releaseFenceKey043711(1)
	recent := releaseFenceKey043711(2)
	live := releaseFenceKey043711(3)
	hard := releaseFenceKey043711(4)
	now := time.Now().Unix()
	state := snapshot{
		Version: 1,
		Bindings: []routingentity.Binding{
			{Kind: "session", Key: live, AccountID: strings.Repeat("a", 32), UpdatedUnix: now},
			{Kind: "previous", Key: hard, AccountID: strings.Repeat("b", 32), UpdatedUnix: now},
		},
		SessionReleaseEpochs: map[string]int64{
			old:    now - 8*24*60*60,
			recent: now - 24*60*60,
			live:   now - 9*24*60*60,
		},
	}
	body, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.snapshotPath(), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Merge(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	got, err := NewStore(store.root).readState()
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := got.SessionReleaseEpochs[old]; kept {
		t.Fatalf("expired retired session fence leaked into compacted snapshot: %+v", got.SessionReleaseEpochs)
	}
	if got.SessionReleaseEpochs[recent] != state.SessionReleaseEpochs[recent] ||
		got.SessionReleaseEpochs[live] != state.SessionReleaseEpochs[live] {
		t.Fatalf("lost recent or live release fence: %+v", got.SessionReleaseEpochs)
	}
	if len(got.Bindings) != 2 {
		t.Fatalf("compaction removed unrelated live session or immutable previous response: %+v", got.Bindings)
	}
	if _, err := os.Stat(store.backupPath()); err != nil {
		t.Fatal(err)
	}
}

// Over time, a one-week fence collection can fill the durable bound. The
// oldest *retired* fences must make room for an explicit new release.
func TestProdex043711ExpiredFenceCapacityDoesNotBlockNewRelease(t *testing.T) {
	store := NewStore(t.TempDir())
	stale := make(map[string]int64, routingentity.MaxBindings)
	now := time.Now().Unix()
	for i := range routingentity.MaxBindings {
		stale[releaseFenceKey043711(i)] = now - 8*24*60*60
	}
	if err := store.writeState(snapshot{Version: 1, SessionReleaseEpochs: stale}); err != nil {
		t.Fatal(err)
	}
	newKey := releaseFenceKey043711(routingentity.MaxBindings + 1)
	if err := store.ReleaseSessions(t.Context(), []string{newKey}); err != nil {
		t.Fatalf("retired week-old fences prevented new native session release: %v", err)
	}
	got, err := NewStore(store.root).readState()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SessionReleaseEpochs) != 1 || got.SessionReleaseEpochs[newKey] < now {
		t.Fatalf("retired fences were not compacted: count=%d new=%d", len(got.SessionReleaseEpochs), got.SessionReleaseEpochs[newKey])
	}
}

func TestProdex043711FenceCompactionPreservesBoundaryLiveAndNewest(t *testing.T) {
	const now = int64(2_000_000)
	const window = sessionReleaseRetiredRetention043711
	live, boundary, expired, future := releaseFenceKey043711(11),
		releaseFenceKey043711(12), releaseFenceKey043711(13), releaseFenceKey043711(14)
	state := snapshot{
		Bindings: []routingentity.Binding{{
			Kind: "session", Key: live,
			AccountID: strings.Repeat("a", 32), UpdatedUnix: now,
		}},
		SessionReleaseEpochs: map[string]int64{
			live: now - 2*window, boundary: now - window,
			expired: now - window - 1, future: now + 1,
		},
	}
	if err := compactSessionReleaseFences043711(&state, now, routingentity.MaxBindings); err != nil {
		t.Fatal(err)
	}
	if len(state.SessionReleaseEpochs) != 3 ||
		state.SessionReleaseEpochs[live] != now-2*window ||
		state.SessionReleaseEpochs[boundary] != now-window ||
		state.SessionReleaseEpochs[future] != now+1 {
		t.Fatalf("wrong live/boundary/future epoch preservation: %+v", state.SessionReleaseEpochs)
	}
	if _, found := state.SessionReleaseEpochs[expired]; found {
		t.Fatalf("expired retired fence was retained: %+v", state.SessionReleaseEpochs)
	}
}

func TestProdex043711FenceCapacityEvictsOldestRetiredDeterministically(t *testing.T) {
	const now = int64(2_000_000)
	a, b, c, d, live := releaseFenceKey043711(21), releaseFenceKey043711(22),
		releaseFenceKey043711(23), releaseFenceKey043711(24), releaseFenceKey043711(25)
	state := snapshot{
		Bindings: []routingentity.Binding{{
			Kind: "session", Key: live, AccountID: strings.Repeat("a", 32), UpdatedUnix: now,
		}},
		SessionReleaseEpochs: map[string]int64{
			a: now - 20, b: now - 10, c: now - 10, d: now - 5,
			live: now - sessionReleaseRetiredRetention043711 - 5,
		},
	}
	if err := compactSessionReleaseFences043711(&state, now, 3); err != nil {
		t.Fatal(err)
	}
	if len(state.SessionReleaseEpochs) != 3 ||
		state.SessionReleaseEpochs[live] == 0 ||
		state.SessionReleaseEpochs[c] == 0 ||
		state.SessionReleaseEpochs[d] == 0 {
		t.Fatalf("failed to preserve live/newest fences: %+v", state.SessionReleaseEpochs)
	}
	if _, exists := state.SessionReleaseEpochs[a]; exists {
		t.Fatal("oldest retired fence retained")
	}
	if _, exists := state.SessionReleaseEpochs[b]; exists {
		t.Fatal("lexically first equal-epoch retired fence retained")
	}
}

func TestProdex043711NearCapacityFreshRetiredFencesMakeRoom(t *testing.T) {
	store := NewStore(t.TempDir())
	now := time.Now().Unix()
	epochs := make(map[string]int64, routingentity.MaxBindings)
	for i := range routingentity.MaxBindings {
		epochs[releaseFenceKey043711(i)] = now - 24*60*60
	}
	if err := store.writeState(snapshot{SessionReleaseEpochs: epochs}); err != nil {
		t.Fatal(err)
	}
	key := releaseFenceKey043711(routingentity.MaxBindings + 1)
	if err := store.ReleaseSessions(t.Context(), []string{key}); err != nil {
		t.Fatalf("new release should evict one retired fence when full: %v", err)
	}
	got, err := NewStore(store.root).readState()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SessionReleaseEpochs) != routingentity.MaxBindings ||
		got.SessionReleaseEpochs[key] < now {
		t.Fatalf("new release lost: count=%d new=%d", len(got.SessionReleaseEpochs), got.SessionReleaseEpochs[key])
	}
	if _, found := got.SessionReleaseEpochs[releaseFenceKey043711(0)]; found {
		t.Fatal("oldest retired key should be evicted at full capacity")
	}
	if _, found := got.SessionReleaseEpochs[releaseFenceKey043711(1)]; !found {
		t.Fatal("next retired key unexpectedly evicted")
	}
}
