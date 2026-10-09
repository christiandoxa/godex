package quota

import (
	"context"
	"errors"
	"sync"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type queuedUsageSnapshotStore struct {
	store usageSnapshotStore
	mu    sync.Mutex
	queue *usageSnapshotSaveQueue
}

func newQueuedUsageSnapshotStore(store usageSnapshotStore) *queuedUsageSnapshotStore {
	return &queuedUsageSnapshotStore{store: store}
}

func (store *queuedUsageSnapshotStore) Load(
	ctx context.Context,
	accountID string,
) (quotamodel.UsageSnapshot, bool, error) {
	return store.store.Load(ctx, accountID)
}

func (store *queuedUsageSnapshotStore) Save(
	ctx context.Context,
	accountID string,
	snapshot quotamodel.UsageSnapshot,
) error {
	queue := store.saveQueue()
	_, err := queue.enqueue(ctx, accountID, snapshot)
	if errors.Is(err, errUsageSnapshotSaveBackpressure) {
		_, err = queue.enqueueWaiting(ctx, accountID, snapshot)
	}
	return err
}

func (store *queuedUsageSnapshotStore) shutdown(ctx context.Context) error {
	store.mu.Lock()
	queue := store.queue
	store.mu.Unlock()
	if queue == nil {
		return nil
	}
	return queue.shutdown(ctx)
}

func (store *queuedUsageSnapshotStore) saveQueue() *usageSnapshotSaveQueue {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.queue == nil {
		store.queue = newUsageSnapshotSaveQueue(func(
			ctx context.Context,
			job usageSnapshotSaveJob,
		) error {
			return store.store.Save(ctx, job.accountID, job.snapshot)
		}, usageSnapshotSaveQueueCapacity, usageSnapshotSaveDebounce)
	}
	return store.queue
}
