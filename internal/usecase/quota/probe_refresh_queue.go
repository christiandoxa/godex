package quota

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	ErrProbeRefreshBackpressure = errors.New("probe refresh queue is at capacity")
	ErrProbeRefreshClosed       = errors.New("probe refresh queue is closed")
)

const (
	defaultProbeRefreshWorkers       = 4
	defaultProbeRefreshQueueCapacity = 64
	defaultProbeRefreshPressureLimit = 16
)

// ProbeRefreshOptions controls the bounded background probe queue. Each job is
// attempted once; pressure is reported separately from hard capacity.
type ProbeRefreshOptions struct {
	WorkerCount   int
	QueueCapacity int
	PressureLimit int
}

type probeRefreshJob struct {
	key string
	run func(context.Context) error
}

// ProbeRefreshQueue deduplicates profile refreshes and owns their workers.
// Workers start lazily and stop when Shutdown cancels the queue context.
type ProbeRefreshQueue struct {
	ctx    context.Context
	cancel context.CancelFunc
	jobs   chan probeRefreshJob
	policy ProbeRefreshOptions

	mu        sync.Mutex
	closed    bool
	pending   int
	scheduled map[string]struct{}
	workers   sync.WaitGroup

	active   atomic.Int64
	revision atomic.Uint64

	progressMu sync.Mutex
	progressCh chan struct{}
	shutdown   sync.Once
	done       chan struct{}
}

func NewProbeRefreshQueue(parent context.Context, options ProbeRefreshOptions) *ProbeRefreshQueue {
	if parent == nil {
		parent = context.Background()
	}
	options = normalizeProbeRefreshOptions(options)
	ctx, cancel := context.WithCancel(parent)
	queue := &ProbeRefreshQueue{
		ctx: ctx, cancel: cancel, jobs: make(chan probeRefreshJob, options.QueueCapacity),
		policy: options, scheduled: make(map[string]struct{}), progressCh: make(chan struct{}),
		done: make(chan struct{}),
	}
	go queue.watchCancellation()
	return queue
}

func normalizeProbeRefreshOptions(options ProbeRefreshOptions) ProbeRefreshOptions {
	if options.WorkerCount <= 0 {
		options.WorkerCount = defaultProbeRefreshWorkers
	}
	if options.QueueCapacity <= 0 {
		options.QueueCapacity = defaultProbeRefreshQueueCapacity
	}
	if options.PressureLimit <= 0 || options.PressureLimit > options.QueueCapacity {
		options.PressureLimit = defaultProbeRefreshPressureLimit
		if options.PressureLimit > options.QueueCapacity {
			options.PressureLimit = options.QueueCapacity
		}
	}
	return options
}

// Schedule accepts one pending job per key. Pressure is an admission signal for
// callers, so jobs continue to be admitted above the pressure limit until the
// separate hard queue capacity is reached. A duplicate already in flight is a
// no-op.
func (queue *ProbeRefreshQueue) Schedule(ctx context.Context, key string, run func(context.Context) error) error {
	if queue == nil {
		return ErrProbeRefreshClosed
	}
	if ctx == nil {
		return errors.New("probe refresh context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	key = strings.TrimSpace(key)
	if key == "" || run == nil {
		return errors.New("probe refresh key and callback are required")
	}
	if err := queue.ctx.Err(); err != nil {
		return err
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return ErrProbeRefreshClosed
	}
	if _, exists := queue.scheduled[key]; exists {
		return nil
	}
	if queue.pending >= queue.policy.QueueCapacity {
		return ErrProbeRefreshBackpressure
	}
	queue.scheduled[key] = struct{}{}
	queue.pending++
	queue.startWorkersLocked()
	select {
	case queue.jobs <- probeRefreshJob{key: key, run: run}:
		return nil
	default:
		queue.pending--
		delete(queue.scheduled, key)
		return ErrProbeRefreshBackpressure
	}
}

func (queue *ProbeRefreshQueue) startWorkersLocked() {
	for index := 0; index < queue.policy.WorkerCount; index++ {
		queue.workers.Add(1)
		go queue.worker()
	}
	// WorkerCount is applied once per queue; a negative pending value is not
	// possible, so this marker also avoids storing another lifecycle counter.
	queue.policy.WorkerCount = 0
}

func (queue *ProbeRefreshQueue) worker() {
	defer queue.workers.Done()
	for {
		select {
		case <-queue.ctx.Done():
			return
		case job := <-queue.jobs:
			queue.mu.Lock()
			if queue.pending > 0 {
				queue.pending--
			}
			closed := queue.closed
			queue.mu.Unlock()
			if closed {
				queue.finish(job.key)
				continue
			}
			queue.active.Add(1)
			queue.execute(job)
			queue.active.Add(-1)
			queue.finish(job.key)
		}
	}
}

func (queue *ProbeRefreshQueue) execute(job probeRefreshJob) {
	if queue.ctx.Err() != nil {
		return
	}
	_ = runProbeSafely(queue.ctx, job.run)
}

func runProbeSafely(ctx context.Context, run func(context.Context) error) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("probe refresh callback panicked")
		}
	}()
	return run(ctx)
}
