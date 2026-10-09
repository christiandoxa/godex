package quota

import (
	"container/list"
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

var (
	errUsageSnapshotSaveBackpressure = errors.New("usage snapshot save queue is under pressure")
	errUsageSnapshotSaveClosed       = errors.New("usage snapshot save queue is closed")
)

const (
	usageSnapshotSaveQueueCapacity = 64
	usageSnapshotSaveDebounce      = 150 * time.Millisecond
)

type usageSnapshotSaveJob struct {
	accountID string
	snapshot  quotamodel.UsageSnapshot
	revision  uint64
	readyAt   time.Time
}

type usageSnapshotSaveQueue struct {
	ctx      context.Context
	cancel   context.CancelFunc
	save     func(context.Context, usageSnapshotSaveJob) error
	capacity int
	debounce time.Duration

	mu       sync.Mutex
	wake     chan struct{}
	changed  chan struct{}
	pending  map[string]*list.Element
	order    list.List
	latest   map[string]uint64
	revision uint64
	closed   bool
	done     chan struct{}
}

func newUsageSnapshotSaveQueue(
	save func(context.Context, usageSnapshotSaveJob) error,
	capacity int,
	debounce time.Duration,
) *usageSnapshotSaveQueue {
	if capacity <= 0 {
		capacity = usageSnapshotSaveQueueCapacity
	}
	if debounce < 0 {
		debounce = 0
	}
	ctx, cancel := context.WithCancel(context.Background())
	queue := &usageSnapshotSaveQueue{
		ctx: ctx, cancel: cancel,
		save: save, capacity: capacity, debounce: debounce,
		wake: make(chan struct{}, 1), changed: make(chan struct{}),
		pending: make(map[string]*list.Element), latest: make(map[string]uint64),
		done: make(chan struct{}),
	}
	go queue.run()
	return queue
}

func (queue *usageSnapshotSaveQueue) enqueue(
	ctx context.Context,
	accountID string,
	snapshot quotamodel.UsageSnapshot,
) (uint64, error) {
	if ctx == nil {
		return 0, errors.New("usage snapshot save context is nil")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return 0, errors.New("usage snapshot save account ID is required")
	}

	queue.mu.Lock()
	if queue.closed {
		queue.mu.Unlock()
		return 0, errUsageSnapshotSaveClosed
	}
	element := queue.pending[accountID]
	if element == nil && len(queue.pending) >= queue.capacity {
		queue.mu.Unlock()
		return 0, errUsageSnapshotSaveBackpressure
	}
	queue.revision++
	revision := queue.revision
	queue.latest[accountID] = revision
	if element == nil {
		job := usageSnapshotSaveJob{
			accountID: accountID, snapshot: snapshot,
			revision: revision, readyAt: time.Now().Add(queue.debounce),
		}
		queue.pending[accountID] = queue.order.PushBack(job)
	} else {
		job := element.Value.(usageSnapshotSaveJob)
		job.snapshot = snapshot
		job.revision = revision
		element.Value = job
	}
	queue.mu.Unlock()
	queue.wakeWorker()
	return revision, nil
}

func (queue *usageSnapshotSaveQueue) enqueueWaiting(
	ctx context.Context,
	accountID string,
	snapshot quotamodel.UsageSnapshot,
) (uint64, error) {
	for {
		revision, err := queue.enqueue(ctx, accountID, snapshot)
		if !errors.Is(err, errUsageSnapshotSaveBackpressure) {
			return revision, err
		}
		queue.mu.Lock()
		if queue.closed {
			queue.mu.Unlock()
			return 0, errUsageSnapshotSaveClosed
		}
		if len(queue.pending) < queue.capacity {
			queue.mu.Unlock()
			continue
		}
		changed := queue.changed
		queue.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

func (queue *usageSnapshotSaveQueue) run() {
	defer queue.cancel()
	defer close(queue.done)
	for {
		queue.mu.Lock()
		if queue.closed && queue.order.Len() == 0 {
			queue.mu.Unlock()
			return
		}
		if queue.order.Len() == 0 {
			queue.mu.Unlock()
			<-queue.wake
			continue
		}

		element := queue.order.Front()
		job := element.Value.(usageSnapshotSaveJob)
		wait := time.Until(job.readyAt)
		if wait > 0 && !queue.closed {
			queue.mu.Unlock()
			queue.waitForWork(wait)
			continue
		}
		queue.order.Remove(element)
		delete(queue.pending, job.accountID)
		queue.notifyLocked()
		shouldSave := queue.latest[job.accountID] == job.revision
		queue.mu.Unlock()

		if shouldSave && queue.ctx.Err() == nil {
			runUsageSnapshotSaveSafely(queue.save, queue.ctx, job)
		}

		queue.mu.Lock()
		if queue.latest[job.accountID] == job.revision {
			delete(queue.latest, job.accountID)
		}
		queue.mu.Unlock()
	}
}

func runUsageSnapshotSaveSafely(
	save func(context.Context, usageSnapshotSaveJob) error,
	ctx context.Context,
	job usageSnapshotSaveJob,
) {
	defer func() { _ = recover() }()
	_ = save(ctx, job)
}

func (queue *usageSnapshotSaveQueue) waitForWork(wait time.Duration) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-queue.wake:
	case <-timer.C:
	}
}

func (queue *usageSnapshotSaveQueue) shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("usage snapshot save shutdown context is nil")
	}
	queue.mu.Lock()
	queue.closed = true
	queue.notifyLocked()
	queue.mu.Unlock()
	queue.wakeWorker()
	select {
	case <-queue.done:
		return nil
	case <-ctx.Done():
		queue.cancel()
		queue.wakeWorker()
		return ctx.Err()
	}
}

func (queue *usageSnapshotSaveQueue) notifyLocked() {
	close(queue.changed)
	queue.changed = make(chan struct{})
}

func (queue *usageSnapshotSaveQueue) backlog() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return len(queue.pending)
}

func (queue *usageSnapshotSaveQueue) wakeWorker() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}
