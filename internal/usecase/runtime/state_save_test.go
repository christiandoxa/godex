package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type stateSaveCall struct {
	kind string
	mask uint8
	data string
}

type stateSaveFake struct {
	mu    sync.Mutex
	calls []stateSaveCall
	err   error
}

type cancelStateSaveFake struct {
	stateStarted chan struct{}
	stateRelease chan struct{}
	mu           sync.Mutex
	calls        []stateSaveCall
}

func (fake *cancelStateSaveFake) SaveSelected(_ context.Context, state, _ []byte, _ []byte, _ []byte, _ []byte, mask uint8) error {
	select {
	case <-fake.stateStarted:
	default:
		close(fake.stateStarted)
	}
	<-fake.stateRelease
	fake.mu.Lock()
	fake.calls = append(fake.calls, stateSaveCall{kind: "state", mask: mask, data: string(state)})
	fake.mu.Unlock()
	return nil
}

func (fake *cancelStateSaveFake) SaveContinuationJournal(context.Context, []byte, int64) error {
	fake.mu.Lock()
	fake.calls = append(fake.calls, stateSaveCall{kind: "journal"})
	fake.mu.Unlock()
	return nil
}

func (fake *cancelStateSaveFake) snapshot() []stateSaveCall {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]stateSaveCall(nil), fake.calls...)
}

func (fake *stateSaveFake) SaveSelected(_ context.Context, state, _ []byte, _ []byte, _ []byte, _ []byte, mask uint8) error {
	fake.mu.Lock()
	fake.calls = append(fake.calls, stateSaveCall{kind: "state", mask: mask, data: string(state)})
	err := fake.err
	fake.mu.Unlock()
	return err
}

func (fake *stateSaveFake) SaveContinuationJournal(_ context.Context, data []byte, _ int64) error {
	fake.mu.Lock()
	fake.calls = append(fake.calls, stateSaveCall{kind: "journal", data: string(data)})
	err := fake.err
	fake.mu.Unlock()
	return err
}

func (fake *stateSaveFake) snapshot() []stateSaveCall {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]stateSaveCall(nil), fake.calls...)
}

func TestRuntimeStateSavePolicyMatchesProdexMutationScopes(t *testing.T) {
	full := RuntimeStateSaveSections{State: RuntimeStateSaveStateFull, Continuations: true, ProfileScores: true, UsageSnapshots: true, Backoffs: true}
	coreProfile := RuntimeStateSaveSections{State: RuntimeStateSaveStateCore, Continuations: true, ProfileScores: true}
	core := RuntimeStateSaveSections{State: RuntimeStateSaveStateCore, Continuations: true}
	tests := []struct {
		kind     string
		sections RuntimeStateSaveSections
		journal  bool
		hot      bool
	}{
		{MutationFullState, full, false, false},
		{MutationStartupAudit, full, false, false},
		{MutationStartupContinuationMigration, full, false, false},
		{MutationStartupBackoffSoften, RuntimeStateSaveSections{Backoffs: true}, false, false},
		{MutationResponseIDs, coreProfile, true, true},
		{MutationPreviousResponseOwner, coreProfile, true, true},
		{MutationPreviousResponseNegative, coreProfile, false, false},
		{MutationPreviousResponseRelease, coreProfile, true, false},
		{MutationResponseTouch, core, false, true},
		{MutationTurnState, core, true, true},
		{MutationTurnStateTouch, core, false, true},
		{MutationSessionID, core, true, true},
		{MutationSessionTouch, core, false, true},
		{MutationSessionAffinityRelease, core, true, false},
		{MutationCompactLineage, core, true, true},
		{MutationCompactLineageRelease, core, true, true},
		{MutationCompactSessionTouch, core, false, true},
		{MutationCompactTurnStateTouch, core, false, true},
		{MutationDeadResponseBindingClear, core, true, false},
		{MutationQuotaRelease, core, true, false},
		{MutationAuthFailedRelease, full, true, false},
		{MutationContinuationStale, core, false, false},
		{MutationProfileCommit, RuntimeStateSaveSections{State: RuntimeStateSaveStateCore, ProfileScores: true, Backoffs: true}, false, false},
		{MutationUsageSnapshot, RuntimeStateSaveSections{UsageSnapshots: true, Backoffs: true}, false, false},
		{MutationProfileRetryBackoff, RuntimeStateSaveSections{UsageSnapshots: true, Backoffs: true}, false, false},
		{MutationProfileTransportBackoff, RuntimeStateSaveSections{Backoffs: true}, false, false},
		{MutationProfileCircuitHalfOpenProbe, RuntimeStateSaveSections{Backoffs: true}, false, false},
		{MutationProfileHealth, RuntimeStateSaveSections{ProfileScores: true, Backoffs: true}, false, false},
		{MutationProfileCircuitClear, RuntimeStateSaveSections{ProfileScores: true, Backoffs: true}, false, false},
		{MutationProfileBadPairing, RuntimeStateSaveSections{ProfileScores: true}, false, false},
		{MutationProfileAuthBackoff, RuntimeStateSaveSections{ProfileScores: true}, false, false},
		{MutationProfileAuthBackoffCleared, RuntimeStateSaveSections{ProfileScores: true}, false, false},
	}
	for _, test := range tests {
		mutation := RuntimeStateMutation{Kind: test.kind, Value: "main"}
		plan, err := RuntimeStateSaveSchedule(mutation, 150*time.Millisecond)
		if err != nil || plan.Sections != test.sections || plan.RequiresContinuationJournal != test.journal {
			t.Fatalf("%s plan = %#v, %v", test.kind, plan, err)
		}
		if (plan.Debounce > 0) != test.hot {
			t.Fatalf("%s debounce = %v", test.kind, plan.Debounce)
		}
	}
}

func TestRuntimeQueuePressureUsesBoundedPendingBacklog(t *testing.T) {
	thresholds := QueuePressureThresholds{StateSave: 8, ContinuationJournal: 8}
	if !RuntimeQueuePressureActive(QueueStats{StatePending: 8}, thresholds) ||
		!RuntimeQueuePressureActive(QueueStats{ContinuationJournalPending: 8}, thresholds) {
		t.Fatal("threshold pressure was not reported")
	}
	if RuntimeQueuePressureActive(QueueStats{StatePending: 7, ContinuationJournalPending: 7}, thresholds) {
		t.Fatal("sub-threshold queues reported pressure")
	}
	if RuntimeQueueEnqueueBacklog(0) != 0 || RuntimeQueueEnqueueBacklog(1) != 0 || RuntimeQueueEnqueueBacklog(3) != 2 {
		t.Fatal("enqueue backlog policy changed")
	}
}

func TestStateSaveSchedulerCoalescesDebouncedStateAndJournalsInOrder(t *testing.T) {
	fake := &stateSaveFake{}
	scheduler := NewStateSaveScheduler(context.Background(), fake, SchedulerOptions{Debounce: time.Hour, QueueCapacity: 2})
	if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
		Key: "root", Mutation: RuntimeStateMutation{Kind: MutationResponseIDs, Value: "first"},
		State: []byte(`{"active":"first"}`), ContinuationData: []byte(`{"response":"first"}`), SavedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
		Key: "root", Mutation: RuntimeStateMutation{Kind: MutationProfileHealth, Value: "first"},
		State: []byte(`{"active":"latest","bindings":{"response":"first"}}`), ProfileScores: []byte(`{"first":{"score":2}}`), SavedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := fake.snapshot()
	if len(calls) != 2 || calls[0].kind != "state" || calls[1].kind != "journal" {
		t.Fatalf("save order/calls = %#v", calls)
	}
	if calls[0].data != `{"active":"latest","bindings":{"response":"first"}}` || calls[0].mask != 0x2d {
		t.Fatalf("coalesced state = %#v", calls[0])
	}
	if calls[1].data != `{"response":"first"}` {
		t.Fatalf("journal payload = %#v", calls[1])
	}
	if got := (RuntimeStateMutation{Kind: MutationProfileAuthBackoffCleared}).Reason(); got != "profile_auth_backoff_cleared:" {
		t.Fatalf("empty mutation reason = %q", got)
	}
}

func TestStateSaveSchedulerJournalCoalescingKeepsContinuationPayload(t *testing.T) {
	fake := &stateSaveFake{}
	scheduler := NewStateSaveScheduler(context.Background(), fake, SchedulerOptions{Debounce: time.Hour})
	if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
		Key: "root", Mutation: RuntimeStateMutation{Kind: MutationTurnState, Value: "first"},
		State: []byte(`{"state":"first"}`), ContinuationData: []byte(`{"turn":"first"}`), SavedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
		Key: "root", Mutation: RuntimeStateMutation{Kind: MutationSessionID, Value: "second"},
		State: []byte(`{"state":"second"}`), SavedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := fake.snapshot()
	if len(calls) != 2 || calls[1].kind != "journal" || calls[1].data != `{"turn":"first"}` {
		t.Fatalf("coalesced journal payload = %#v", calls)
	}
}

func TestStateSaveSchedulerDispatchesDuePathsInSortedOrder(t *testing.T) {
	fake := &stateSaveFake{}
	scheduler := NewStateSaveScheduler(context.Background(), fake, SchedulerOptions{Debounce: time.Hour, QueueCapacity: 3})
	for _, key := range []string{"z", "a", "m"} {
		if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
			Key: key, Mutation: RuntimeStateMutation{Kind: MutationResponseTouch, Value: key}, State: []byte(key),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := scheduler.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := fake.snapshot()
	if len(calls) != 3 || calls[0].data != "a" || calls[1].data != "m" || calls[2].data != "z" {
		t.Fatalf("sorted jobs = %#v", calls)
	}
}

func TestStateSaveSchedulerBoundsQueueAndReportsSaveError(t *testing.T) {
	fake := &stateSaveFake{err: errors.New("save failed")}
	scheduler := NewStateSaveScheduler(context.Background(), fake, SchedulerOptions{Debounce: time.Hour, QueueCapacity: 1})
	request := StateSaveRequest{Mutation: RuntimeStateMutation{Kind: MutationResponseTouch, Value: "a"}, State: []byte(`{}`)}
	request.Key = "a"
	if err := scheduler.Enqueue(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Key = "b"
	if err := scheduler.Enqueue(context.Background(), request); !errors.Is(err, ErrStateSaveQueueFull) {
		t.Fatalf("second key error = %v", err)
	}
	request.Key = "a"
	if err := scheduler.Enqueue(context.Background(), request); err != nil {
		t.Fatalf("coalescing full key = %v", err)
	}
	if err := scheduler.Flush(context.Background()); err == nil || err.Error() != "save failed" {
		t.Fatalf("flush error = %v", err)
	}
	if err := scheduler.Shutdown(context.Background()); err == nil {
		t.Fatal("shutdown hid save failure")
	}
}

func TestStateSaveSchedulerContextCancellationDropsPendingWork(t *testing.T) {
	fake := &stateSaveFake{}
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := NewStateSaveScheduler(ctx, fake, SchedulerOptions{Debounce: time.Hour})
	if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
		Key: "root", Mutation: RuntimeStateMutation{Kind: MutationResponseTouch, Value: "root"}, State: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := scheduler.Shutdown(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled shutdown error = %v", err)
	}
	if calls := fake.snapshot(); len(calls) != 0 {
		t.Fatalf("canceled scheduler wrote %d calls", len(calls))
	}
}

func TestStateSaveSchedulerFlushReportsParentCancellation(t *testing.T) {
	fake := &stateSaveFake{}
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := NewStateSaveScheduler(ctx, fake, SchedulerOptions{Debounce: time.Hour})
	if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
		Key: "root", Mutation: RuntimeStateMutation{Kind: MutationResponseTouch, Value: "root"}, State: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := scheduler.Flush(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled flush error = %v", err)
	}
	if calls := fake.snapshot(); len(calls) != 0 {
		t.Fatalf("canceled flush wrote %d calls", len(calls))
	}
}

func TestStateSaveSchedulerCancellationSkipsJobsAlreadyDequeued(t *testing.T) {
	fake := &cancelStateSaveFake{stateStarted: make(chan struct{}), stateRelease: make(chan struct{})}
	scheduler := NewStateSaveScheduler(context.Background(), fake, SchedulerOptions{Debounce: time.Hour, QueueCapacity: 2})
	for _, key := range []string{"a", "b"} {
		if err := scheduler.Enqueue(context.Background(), StateSaveRequest{
			Key: key, Mutation: RuntimeStateMutation{Kind: MutationProfileCommit, Value: key}, State: []byte(key),
		}); err != nil {
			t.Fatal(err)
		}
	}
	<-fake.stateStarted
	scheduler.Cancel()
	close(fake.stateRelease)
	if err := scheduler.Shutdown(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled shutdown error = %v", err)
	}
	if calls := fake.snapshot(); len(calls) != 1 || calls[0].data != "a" {
		t.Fatalf("dequeued cancellation calls = %#v", calls)
	}
}
