package runtime

import (
	"sort"
	"time"
)

func (scheduler *StateSaveScheduler) run() {
	defer close(scheduler.done)
	for {
		scheduler.mu.Lock()
		if scheduler.ctx.Err() != nil {
			scheduler.canceled = true
			scheduler.closing = true
			clear(scheduler.state)
			clear(scheduler.journal)
		}
		if scheduler.canceled {
			scheduler.finishFlushersLocked()
			scheduler.mu.Unlock()
			return
		}
		stateJobs, journalJobs, wait := scheduler.takeReadyLocked(time.Now())
		if len(stateJobs) == 0 && len(journalJobs) == 0 {
			if scheduler.closing && len(scheduler.state) == 0 && len(scheduler.journal) == 0 {
				scheduler.finishFlushersLocked()
				scheduler.mu.Unlock()
				return
			}
			if scheduler.flush && len(scheduler.state) == 0 && len(scheduler.journal) == 0 {
				scheduler.finishFlushersLocked()
			}
			if len(scheduler.state) == 0 && len(scheduler.journal) == 0 {
				wait = time.Second
			}
			scheduler.mu.Unlock()
			scheduler.wait(wait)
			continue
		}
		scheduler.mu.Unlock()
		for _, job := range stateJobs {
			if scheduler.stopped() {
				break
			}
			scheduler.runState(job)
		}
		if scheduler.stopped() {
			continue
		}
		for _, job := range journalJobs {
			if scheduler.stopped() {
				break
			}
			scheduler.runJournal(job)
		}
	}
}

func (scheduler *StateSaveScheduler) stopped() bool {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return scheduler.canceled || scheduler.ctx.Err() != nil
}

func (scheduler *StateSaveScheduler) takeReadyLocked(now time.Time) ([]stateSaveJob, []journalSaveJob, time.Duration) {
	ready := func(at time.Time) bool { return scheduler.flush || !at.After(now) }
	var stateJobs []stateSaveJob
	var journalJobs []journalSaveJob
	next := time.Duration(0)
	stateKeys := make([]string, 0, len(scheduler.state))
	for key := range scheduler.state {
		stateKeys = append(stateKeys, key)
	}
	sort.Strings(stateKeys)
	for _, key := range stateKeys {
		job := scheduler.state[key]
		if ready(job.readyAt) {
			stateJobs = append(stateJobs, job)
			delete(scheduler.state, key)
			continue
		}
		next = minWait(next, time.Until(job.readyAt))
	}
	journalKeys := make([]string, 0, len(scheduler.journal))
	for key := range scheduler.journal {
		journalKeys = append(journalKeys, key)
	}
	sort.Strings(journalKeys)
	for _, key := range journalKeys {
		job := scheduler.journal[key]
		if ready(job.readyAt) {
			journalJobs = append(journalJobs, job)
			delete(scheduler.journal, key)
			continue
		}
		next = minWait(next, time.Until(job.readyAt))
	}
	return stateJobs, journalJobs, next
}

func (scheduler *StateSaveScheduler) runState(job stateSaveJob) {
	scheduler.mu.Lock()
	scheduler.activeState++
	scheduler.mu.Unlock()
	mask := sectionMask(job.sections)
	err := scheduler.saver.SaveSelected(scheduler.ctx, job.request.State, job.request.Continuations, job.request.ProfileScores, job.request.UsageSnapshots, job.request.Backoffs, mask)
	scheduler.recordError(err)
	scheduler.mu.Lock()
	scheduler.activeState--
	scheduler.mu.Unlock()
}

func (scheduler *StateSaveScheduler) runJournal(job journalSaveJob) {
	scheduler.mu.Lock()
	scheduler.activeJournal++
	scheduler.mu.Unlock()
	err := scheduler.saver.SaveContinuationJournal(scheduler.ctx, job.request.ContinuationData, job.request.SavedAt)
	scheduler.recordError(err)
	scheduler.mu.Lock()
	scheduler.activeJournal--
	scheduler.mu.Unlock()
}

func (scheduler *StateSaveScheduler) recordError(err error) {
	if err == nil {
		return
	}
	scheduler.mu.Lock()
	if scheduler.saveErr == nil {
		scheduler.saveErr = err
	}
	if scheduler.flushErr == nil {
		scheduler.flushErr = err
	}
	scheduler.mu.Unlock()
}

func (scheduler *StateSaveScheduler) shutdownError() error {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return scheduler.saveErr
}

func (scheduler *StateSaveScheduler) finishFlushersLocked() {
	if !scheduler.flush {
		return
	}
	scheduler.flush = false
	err := scheduler.flushErr
	if err == nil && scheduler.canceled {
		err = scheduler.ctx.Err()
		if err == nil {
			err = ErrStateSaveClosed
		}
	}
	scheduler.flushErr = nil
	for _, waiter := range scheduler.waiters {
		waiter.done <- err
	}
	scheduler.waiters = nil
}

func (scheduler *StateSaveScheduler) wait(wait time.Duration) {
	if wait < 0 {
		wait = time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-scheduler.wake:
	case <-scheduler.ctx.Done():
		scheduler.mu.Lock()
		scheduler.canceled = true
		scheduler.closing = true
		clear(scheduler.state)
		clear(scheduler.journal)
		scheduler.mu.Unlock()
	}
}

func (scheduler *StateSaveScheduler) notify() {
	select {
	case scheduler.wake <- struct{}{}:
	default:
	}
}

func sectionMask(sections RuntimeStateSaveSections) uint8 {
	mask := uint8(sections.State)
	if sections.Continuations {
		mask |= 0x04
	}
	if sections.ProfileScores {
		mask |= 0x08
	}
	if sections.UsageSnapshots {
		mask |= 0x10
	}
	if sections.Backoffs {
		mask |= 0x20
	}
	return mask
}

func cloneRequest(request StateSaveRequest) StateSaveRequest {
	request.State = append([]byte(nil), request.State...)
	request.Continuations = append([]byte(nil), request.Continuations...)
	request.ProfileScores = append([]byte(nil), request.ProfileScores...)
	request.UsageSnapshots = append([]byte(nil), request.UsageSnapshots...)
	request.Backoffs = append([]byte(nil), request.Backoffs...)
	request.ContinuationData = append([]byte(nil), request.ContinuationData...)
	return request
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func minWait(current, next time.Duration) time.Duration {
	if next < 0 {
		return 0
	}
	if current == 0 || next < current {
		return next
	}
	return current
}
