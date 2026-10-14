package routing

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
)

func TestProdex043711AbsentReleaseFenceMonotoneAndKeepsHardResponse(t *testing.T) {
	repo := NewStore(t.TempDir())
	ctx := t.Context()
	session := strings.Repeat("a", 64)
	compact := strings.Repeat("b", 64)
	previous := routingentity.Binding{
		Kind: "previous", Key: strings.Repeat("c", 64),
		AccountID: strings.Repeat("1", 32), UpdatedUnix: time.Now().Unix(),
	}
	if _, err := repo.Merge(ctx, []routingentity.Binding{previous}); err != nil {
		t.Fatal(err)
	}
	var lastSession, lastCompact int64
	for range 3 {
		if err := repo.ReleaseSessions(ctx, []string{session, compact}); err != nil {
			t.Fatal(err)
		}
		state, err := NewStore(repo.root).readState()
		if err != nil {
			t.Fatal(err)
		}
		if state.SessionReleaseEpochs[session] <= lastSession ||
			state.SessionReleaseEpochs[compact] <= lastCompact {
			t.Fatalf("same-second absent release did not advance: %+v", state.SessionReleaseEpochs)
		}
		lastSession = state.SessionReleaseEpochs[session]
		lastCompact = state.SessionReleaseEpochs[compact]
		if len(state.Bindings) != 1 || state.Bindings[0] != previous {
			t.Fatalf("hard response was altered by session release: %+v", state.Bindings)
		}
	}
	if err := repo.ReleaseSessions(ctx, []string{strings.Repeat("g", 64)}); err == nil {
		t.Fatal("invalid session release digest accepted")
	}
	if err := repo.ReleaseSessions(ctx, []string{session, "unsafe"}); err == nil {
		t.Fatal("partial invalid session release accepted")
	}
	state, err := repo.readState()
	if err != nil {
		t.Fatal(err)
	}
	if state.SessionReleaseEpochs[session] != lastSession ||
		state.SessionReleaseEpochs[compact] != lastCompact {
		t.Fatalf("invalid release partially changed persisted epochs: %+v", state.SessionReleaseEpochs)
	}
	// Existing release fences must not be overwritten by a partial
	// invalid primary snapshot, or its last-good fallback.
	if err := os.WriteFile(filepath.Join(repo.root, "routing.json"),
		[]byte(`{"version":1,"bindings":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(repo.root).readState()
	if err != nil || reloaded.SessionReleaseEpochs[session] != lastSession {
		t.Fatalf("lost retired fence during backup repair: %+v err=%v", reloaded.SessionReleaseEpochs, err)
	}
}

func TestProdex043711ConcurrentStaleWorkersCannotReintroduceReleasedOwner(t *testing.T) {
	repo := NewStore(t.TempDir())
	const key = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	old := routingentity.Binding{
		Kind: "session", Key: key,
		AccountID:   strings.Repeat("1", 32),
		UpdatedUnix: time.Now().Add(-time.Hour).Unix(),
	}
	if err := repo.ReleaseSessions(t.Context(), []string{key}); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := NewStore(repo.root).Merge(t.Context(), []routingentity.Binding{old}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	loaded, err := NewStore(repo.root).Load(t.Context())
	if err != nil || len(loaded) != 0 {
		t.Fatalf("stale workers resurrected session: %+v err=%v", loaded, err)
	}
}
