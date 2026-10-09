package quota

import (
	"context"
)

func (queue *ProbeRefreshQueue) watchCancellation() {
	<-queue.ctx.Done()
	queue.mu.Lock()
	if queue.closed {
		queue.mu.Unlock()
		return
	}
	queue.closed = true
	pending := queue.pending
	queue.pending = 0
	clear(queue.scheduled)
	queue.mu.Unlock()
	if pending > 0 {
		queue.noteProgress()
	}
}

func (queue *ProbeRefreshQueue) finish(key string) {
	queue.mu.Lock()
	delete(queue.scheduled, key)
	queue.mu.Unlock()
	queue.noteProgress()
}

func (queue *ProbeRefreshQueue) noteProgress() {
	queue.progressMu.Lock()
	queue.revision.Add(1)
	close(queue.progressCh)
	queue.progressCh = make(chan struct{})
	queue.progressMu.Unlock()
}

func (queue *ProbeRefreshQueue) Backlog() int {
	if queue == nil {
		return 0
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.pending
}

func (queue *ProbeRefreshQueue) Active() int {
	if queue == nil {
		return 0
	}
	return int(queue.active.Load())
}

func (queue *ProbeRefreshQueue) Pressure() bool {
	if queue == nil {
		return false
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.pending >= queue.policy.PressureLimit
}

func (queue *ProbeRefreshQueue) Revision() uint64 {
	if queue == nil {
		return 0
	}
	return queue.revision.Load()
}

// WaitProgress waits for completion, panic recovery, or cancellation progress
// after observed. It never treats queue admission or unrelated work as progress.
func (queue *ProbeRefreshQueue) WaitProgress(ctx context.Context, observed uint64) bool {
	if queue == nil || ctx == nil {
		return false
	}
	for {
		queue.progressMu.Lock()
		if queue.revision.Load() != observed {
			queue.progressMu.Unlock()
			return true
		}
		progress := queue.progressCh
		queue.progressMu.Unlock()
		select {
		case <-progress:
		case <-ctx.Done():
			return false
		}
	}
}

// Shutdown cancels queued callbacks and waits for workers to release their
// active keys. A deadline bounds the caller without abandoning worker cleanup.
func (queue *ProbeRefreshQueue) Shutdown(ctx context.Context) error {
	if queue == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queue.shutdown.Do(func() {
		queue.mu.Lock()
		queue.closed = true
		pending := queue.pending
		queue.pending = 0
		clear(queue.scheduled)
		queue.mu.Unlock()
		queue.cancel()
		if pending > 0 {
			queue.noteProgress()
		}
		go func() {
			queue.workers.Wait()
			close(queue.done)
		}()
	})
	select {
	case <-queue.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (queue *ProbeRefreshQueue) Close() { _ = queue.Shutdown(context.Background()) }
