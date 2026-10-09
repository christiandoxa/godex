package quota

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestUsageSnapshotSaveQueueCoalescesToLatestRevision(t *testing.T) {
	var (
		mu   sync.Mutex
		jobs []usageSnapshotSaveJob
	)
	queue := newUsageSnapshotSaveQueue(func(_ context.Context, job usageSnapshotSaveJob) error {
		mu.Lock()
		jobs = append(jobs, job)
		mu.Unlock()
		return nil
	}, 4, time.Hour)
	if _, err := queue.enqueue(context.Background(), "main", testUsageSnapshot(1)); err != nil {
		t.Fatal(err)
	}
	revision, err := queue.enqueue(context.Background(), "main", testUsageSnapshot(2))
	if err != nil {
		t.Fatal(err)
	}
	if revision != 2 || queue.backlog() != 1 {
		t.Fatalf("coalesced revision/backlog = %d/%d", revision, queue.backlog())
	}
	if err := queue.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jobs) != 1 || jobs[0].revision != 2 || jobs[0].snapshot.CheckedAt != 2 {
		t.Fatalf("saved jobs = %#v, want latest revision only", jobs)
	}
}

func TestUsageSnapshotSaveQueuePreservesFIFOForDistinctAccounts(t *testing.T) {
	var (
		mu  sync.Mutex
		ids []string
	)
	queue := newUsageSnapshotSaveQueue(func(_ context.Context, job usageSnapshotSaveJob) error {
		mu.Lock()
		ids = append(ids, job.accountID)
		mu.Unlock()
		return nil
	}, 4, time.Hour)
	for _, id := range []string{"second", "first"} {
		if _, err := queue.enqueue(context.Background(), id, testUsageSnapshot(1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := queue.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got, want := ids, []string{"second", "first"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("save order = %#v, want %#v", got, want)
	}
}

func TestUsageSnapshotSaveQueueBackpressureAndCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	queue := newUsageSnapshotSaveQueue(func(_ context.Context, job usageSnapshotSaveJob) error {
		if job.accountID == "active" {
			close(started)
			<-release
		}
		return nil
	}, 1, 0)
	if _, err := queue.enqueue(context.Background(), "active", testUsageSnapshot(1)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("queue did not start active job")
	}
	if _, err := queue.enqueue(context.Background(), "pending", testUsageSnapshot(2)); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.enqueue(context.Background(), "overflow", testUsageSnapshot(3)); !errors.Is(err, errUsageSnapshotSaveBackpressure) {
		t.Fatalf("backpressure error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := queue.enqueue(ctx, "canceled", testUsageSnapshot(4)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled enqueue error = %v", err)
	}
	close(release)
	if err := queue.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.backlog() != 0 {
		t.Fatalf("queue backlog after shutdown = %d", queue.backlog())
	}
}

func TestUsageSnapshotSaveQueueWritesNewRevisionAfterActiveOlderJob(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var (
		mu   sync.Mutex
		jobs []usageSnapshotSaveJob
	)
	queue := newUsageSnapshotSaveQueue(func(_ context.Context, job usageSnapshotSaveJob) error {
		mu.Lock()
		jobs = append(jobs, job)
		mu.Unlock()
		if job.revision == 1 {
			close(started)
			<-release
		}
		return nil
	}, 4, 0)
	if _, err := queue.enqueue(context.Background(), "main", testUsageSnapshot(1)); err != nil {
		t.Fatal(err)
	}
	<-started
	if revision, err := queue.enqueue(context.Background(), "main", testUsageSnapshot(2)); err != nil || revision != 2 {
		t.Fatalf("new revision = %d, err=%v", revision, err)
	}
	close(release)
	if err := queue.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jobs) != 2 || jobs[0].revision != 1 || jobs[1].revision != 2 || jobs[1].snapshot.CheckedAt != 2 {
		t.Fatalf("saved jobs = %#v, want revisions 1 then 2", jobs)
	}
}

func TestQueuedUsageSnapshotStoreFlushesAtProcessShutdown(t *testing.T) {
	underlying := &memoryUsageSnapshotStore{}
	store := newQueuedUsageSnapshotStore(underlying)
	if err := store.Save(context.Background(), "main", testUsageSnapshot(9)); err != nil {
		t.Fatal(err)
	}
	if _, ok := underlying.values["main"]; ok {
		t.Fatal("queued save wrote before shutdown")
	}
	if err := store.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := underlying.values["main"].CheckedAt; got != 9 {
		t.Fatalf("flushed snapshot checked_at = %d, want 9", got)
	}
}

func testUsageSnapshot(checkedAt int64) quotamodel.UsageSnapshot {
	return quotamodel.UsageSnapshot{CheckedAt: checkedAt}
}
