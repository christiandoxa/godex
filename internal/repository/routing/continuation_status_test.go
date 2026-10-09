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

func TestContinuationStatusPersistsHashedDeadStateAndExpires(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Unix(2_000_000, 0)
	key := strings.Repeat("a", 64)
	status := routingentity.ContinuationStatus{
		Kind: "turn_state", Key: key, State: "dead", UpdatedUnix: now.Unix(),
	}
	if err := store.SaveContinuationStatus(context.Background(), status, now); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadContinuationStatuses(context.Background(), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0] != status {
		t.Fatalf("loaded statuses = %#v, want %#v", loaded, []routingentity.ContinuationStatus{status})
	}
	content, err := os.ReadFile(filepath.Join(root, "continuation-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), key) {
		t.Fatalf("snapshot omitted hashed key: %s", content)
	}
	if strings.Contains(string(content), "opaque-turn-state") {
		t.Fatal("snapshot contains an opaque continuation value")
	}
	loaded, err = store.LoadContinuationStatuses(context.Background(), now.Add(continuationStatusRetention+time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expired statuses = %#v", loaded)
	}
}

func TestContinuationStatusRejectsInvalidSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "continuation-status.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"statuses":[{"kind":"turn_state","key":"`+strings.Repeat("b", 64)+`","state":"warm","updated_unix":1}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(root).LoadContinuationStatuses(context.Background(), time.Unix(2, 0)); err == nil {
		t.Fatal("accepted a non-private continuation status snapshot")
	}
}
