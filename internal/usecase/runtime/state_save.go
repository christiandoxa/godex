package runtime

import (
	"context"
	"errors"
	"sync"
	"time"
)

// StateSaveRequest carries full current snapshots. Mutation controls which
// sections are persisted; it does not turn the byte fields into patches.
type StateSaveRequest struct {
	Key              string
	Mutation         RuntimeStateMutation
	State            []byte
	Continuations    []byte
	ProfileScores    []byte
	UsageSnapshots   []byte
	Backoffs         []byte
	ContinuationData []byte
	SavedAt          int64
}

type SchedulerOptions struct {
	Debounce      time.Duration
	QueueCapacity int
}

type QueueStats struct {
	StatePending               int
	ContinuationJournalPending int
	StateActive                int
	ContinuationJournalActive  int
}

type stateSaveJob struct {
	request  StateSaveRequest
	sections RuntimeStateSaveSections
	readyAt  time.Time
}

type journalSaveJob struct {
	request StateSaveRequest
	readyAt time.Time
}

type flushWaiter struct {
	done chan error
}

// StateSaveScheduler coalesces per-root writes and owns one draining worker.
// Shutdown drains queued work; Cancel discards queued work explicitly.
type StateSaveScheduler struct {
	saver         StateSaver
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	state         map[string]stateSaveJob
	journal       map[string]journalSaveJob
	capacity      int
	debounce      time.Duration
	wake          chan struct{}
	done          chan struct{}
	closing       bool
	canceled      bool
	manualCancel  bool
	flush         bool
	waiters       []*flushWaiter
	saveErr       error
	flushErr      error
	activeState   int
	activeJournal int
}

func NewStateSaveScheduler(ctx context.Context, saver StateSaver, options SchedulerOptions) *StateSaveScheduler {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	if options.Debounce <= 0 {
		options.Debounce = defaultRuntimeStateSaveDebounce
	}
	if options.QueueCapacity <= 0 {
		options.QueueCapacity = 64
	}
	scheduler := &StateSaveScheduler{
		saver: saver, ctx: ctx, cancel: cancel, state: make(map[string]stateSaveJob), journal: make(map[string]journalSaveJob),
		capacity: options.QueueCapacity, debounce: options.Debounce,
		wake: make(chan struct{}, 1), done: make(chan struct{}),
	}
	go scheduler.run()
	return scheduler
}

func (scheduler *StateSaveScheduler) Enqueue(ctx context.Context, request StateSaveRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scheduler == nil || scheduler.saver == nil {
		return errors.New("runtime state saver is not configured")
	}
	plan, err := RuntimeStateSaveSchedule(request.Mutation, scheduler.debounce)
	if err != nil {
		return err
	}
	if request.Key == "" {
		request.Key = "default"
	}
	now := time.Now()
	job := stateSaveJob{request: cloneRequest(request), sections: plan.Sections, readyAt: now.Add(plan.Debounce)}
	scheduler.mu.Lock()
	if err := scheduler.ctx.Err(); err != nil {
		scheduler.canceled = true
		scheduler.closing = true
		clear(scheduler.state)
		clear(scheduler.journal)
		scheduler.mu.Unlock()
		return err
	}
	if scheduler.closing || scheduler.canceled {
		scheduler.mu.Unlock()
		return ErrStateSaveClosed
	}
	if previous, ok := scheduler.state[request.Key]; ok {
		job.request = mergeSaveRequest(previous.request, job.request)
		job.sections = previous.sections.Union(job.sections)
		job.readyAt = minTime(previous.readyAt, job.readyAt)
	}
	_, stateExists := scheduler.state[request.Key]
	_, journalExists := scheduler.journal[request.Key]
	if !stateExists && len(scheduler.state) >= scheduler.capacity {
		scheduler.mu.Unlock()
		return ErrStateSaveQueueFull
	}
	if plan.RequiresContinuationJournal && !journalExists && len(scheduler.journal) >= scheduler.capacity {
		scheduler.mu.Unlock()
		return ErrStateSaveQueueFull
	}
	scheduler.state[request.Key] = job
	if plan.RequiresContinuationJournal {
		journalRequest := cloneRequest(request)
		journal := journalSaveJob{request: journalRequest, readyAt: now.Add(RuntimeStateSaveDebounce(request.Mutation, scheduler.debounce))}
		if previous, ok := scheduler.journal[request.Key]; ok {
			journal.request = mergeSaveRequest(previous.request, journal.request)
			journal.request.SavedAt = max(previous.request.SavedAt, journal.request.SavedAt)
			journal.readyAt = minTime(previous.readyAt, journal.readyAt)
		}
		scheduler.journal[request.Key] = journal
	}
	scheduler.mu.Unlock()
	scheduler.notify()
	return nil
}

// Flush wakes the worker, bypasses debounce, drains both queues, and returns
// the first persistence error observed during this flush.
func (scheduler *StateSaveScheduler) Flush(ctx context.Context) error {
	if scheduler == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	waiter := &flushWaiter{done: make(chan error, 1)}
	scheduler.mu.Lock()
	if scheduler.canceled {
		if !scheduler.manualCancel && scheduler.ctx.Err() != nil {
			err := scheduler.ctx.Err()
			scheduler.mu.Unlock()
			return err
		}
		scheduler.mu.Unlock()
		return ErrStateSaveClosed
	}
	if err := scheduler.ctx.Err(); err != nil {
		scheduler.canceled = true
		scheduler.closing = true
		clear(scheduler.state)
		clear(scheduler.journal)
		scheduler.mu.Unlock()
		return err
	}
	if scheduler.closing && len(scheduler.state) == 0 && len(scheduler.journal) == 0 && scheduler.activeState == 0 && scheduler.activeJournal == 0 {
		err := scheduler.saveErr
		scheduler.mu.Unlock()
		return err
	}
	scheduler.flush = true
	scheduler.waiters = append(scheduler.waiters, waiter)
	scheduler.mu.Unlock()
	scheduler.notify()
	select {
	case err := <-waiter.done:
		return err
	case <-ctx.Done():
		scheduler.mu.Lock()
		for index, pending := range scheduler.waiters {
			if pending == waiter {
				scheduler.waiters = append(scheduler.waiters[:index], scheduler.waiters[index+1:]...)
				break
			}
		}
		scheduler.mu.Unlock()
		return ctx.Err()
	}
}

// Shutdown drains queued jobs before returning. A deadline reports to the
// caller while the owned worker continues draining in the background.
func (scheduler *StateSaveScheduler) Shutdown(ctx context.Context) error {
	if scheduler == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scheduler.mu.Lock()
	scheduler.closing = true
	scheduler.flush = true
	scheduler.mu.Unlock()
	scheduler.notify()
	select {
	case <-scheduler.done:
		scheduler.mu.Lock()
		canceled := scheduler.canceled
		scheduler.mu.Unlock()
		if canceled && scheduler.shutdownError() == nil {
			return context.Canceled
		}
		return scheduler.shutdownError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Cancel abandons pending jobs and stops the worker after any active write.
func (scheduler *StateSaveScheduler) Cancel() {
	if scheduler == nil {
		return
	}
	scheduler.mu.Lock()
	scheduler.canceled = true
	scheduler.manualCancel = true
	scheduler.closing = true
	clear(scheduler.state)
	clear(scheduler.journal)
	scheduler.mu.Unlock()
	scheduler.cancel()
	scheduler.notify()
}

func (scheduler *StateSaveScheduler) Stats() QueueStats {
	if scheduler == nil {
		return QueueStats{}
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return QueueStats{StatePending: len(scheduler.state), ContinuationJournalPending: len(scheduler.journal), StateActive: scheduler.activeState, ContinuationJournalActive: scheduler.activeJournal}
}

func mergeSaveRequest(previous, latest StateSaveRequest) StateSaveRequest {
	latest.State = firstNonempty(latest.State, previous.State)
	latest.Continuations = firstNonempty(latest.Continuations, previous.Continuations)
	latest.ProfileScores = firstNonempty(latest.ProfileScores, previous.ProfileScores)
	latest.UsageSnapshots = firstNonempty(latest.UsageSnapshots, previous.UsageSnapshots)
	latest.Backoffs = firstNonempty(latest.Backoffs, previous.Backoffs)
	if len(latest.ContinuationData) == 0 {
		latest.ContinuationData = previous.ContinuationData
	}
	if latest.SavedAt < previous.SavedAt {
		latest.SavedAt = previous.SavedAt
	}
	return latest
}

func firstNonempty(latest, previous []byte) []byte {
	if len(latest) == 0 {
		return append([]byte(nil), previous...)
	}
	return latest
}
