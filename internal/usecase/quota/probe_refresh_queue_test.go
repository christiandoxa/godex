package quota

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProbeRefreshQueueDeduplicatesAndReportsCompletion(t *testing.T) {
	queue := NewProbeRefreshQueue(context.Background(), ProbeRefreshOptions{WorkerCount: 1, PressureLimit: 4})
	defer queue.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	observed := queue.Revision()
	run := func(context.Context) error {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return nil
	}
	if err := queue.Schedule(context.Background(), "main", run); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := queue.Schedule(context.Background(), "main", run); err != nil {
		t.Fatalf("duplicate schedule error = %v", err)
	}
	if got := queue.Active(); got != 1 {
		t.Fatalf("active probes = %d, want 1", got)
	}
	close(release)
	if !queue.WaitProgress(context.Background(), observed) {
		t.Fatal("queue completion did not notify progress")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("callback calls = %d, want 1", got)
	}
	if queue.Backlog() != 0 || queue.Active() != 0 {
		t.Fatalf("queue did not release resources: backlog=%d active=%d", queue.Backlog(), queue.Active())
	}
}

func TestProbeRefreshQueueShedsPressureAndReusesKeyAfterFailure(t *testing.T) {
	queue := NewProbeRefreshQueue(context.Background(), ProbeRefreshOptions{WorkerCount: 1, PressureLimit: 2, QueueCapacity: 8})
	defer queue.Close()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	done := make(chan struct{}, 3)
	var calls atomic.Int32
	run := func(context.Context) error {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		done <- struct{}{}
		return errors.New("provider failed")
	}
	if err := queue.Schedule(context.Background(), "a", run); err != nil {
		t.Fatal(err)
	}
	<-firstStarted
	if err := queue.Schedule(context.Background(), "b", run); err != nil {
		t.Fatal(err)
	}
	if err := queue.Schedule(context.Background(), "c", run); err != nil {
		t.Fatal(err)
	}
	if !queue.Pressure() {
		t.Fatal("queue did not report pending pressure")
	}
	if err := queue.Schedule(context.Background(), "d", run); !errors.Is(err, ErrProbeRefreshBackpressure) {
		t.Fatalf("pressure schedule error = %v, want ErrProbeRefreshBackpressure", err)
	}
	close(releaseFirst)
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for completed := 0; completed < 3; completed++ {
		select {
		case <-deadline.C:
			t.Fatalf("queued failures did not drain; calls=%d", calls.Load())
		case <-done:
		}
	}
	if err := queue.Schedule(context.Background(), "a", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("completed key was not released: %v", err)
	}
}

func TestProbeRefreshQueueRetryStopsOnShutdownCancellation(t *testing.T) {
	queue := NewProbeRefreshQueue(context.Background(), ProbeRefreshOptions{
		WorkerCount: 1, MaxAttempts: 3, RetryDelay: time.Second, MaxRetryDelay: time.Second,
	})
	started := make(chan struct{})
	var calls atomic.Int32
	run := func(context.Context) error {
		calls.Add(1)
		close(started)
		return probeRefreshTimeoutError{}
	}
	if err := queue.Schedule(context.Background(), "main", run); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := queue.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("retry calls after cancellation = %d, want 1", got)
	}
	if queue.Active() != 0 || queue.Backlog() != 0 {
		t.Fatalf("shutdown leaked queue resources: backlog=%d active=%d", queue.Backlog(), queue.Active())
	}
}

func TestProbeRefreshQueueRetriesTransientFailureWithBoundedAttempts(t *testing.T) {
	queue := NewProbeRefreshQueue(context.Background(), ProbeRefreshOptions{
		WorkerCount: 1, MaxAttempts: 3, RetryDelay: time.Nanosecond, MaxRetryDelay: time.Nanosecond,
	})
	defer queue.Close()
	var calls atomic.Int32
	second := make(chan struct{})
	observed := queue.Revision()
	if err := queue.Schedule(context.Background(), "main", func(context.Context) error {
		if calls.Add(1) == 1 {
			return probeRefreshTimeoutError{}
		}
		close(second)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("transient probe failure was not retried")
	}
	if !queue.WaitProgress(context.Background(), observed) {
		t.Fatal("retry completion did not report progress")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("retry calls = %d, want 2", got)
	}
}

func TestProbeRefreshQueueParentCancellationDropsPendingJobs(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	queue := NewProbeRefreshQueue(parent, ProbeRefreshOptions{WorkerCount: 1, PressureLimit: 4})
	started := make(chan struct{})
	if err := queue.Schedule(context.Background(), "active", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := queue.Schedule(context.Background(), "pending", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	observed := queue.Revision()
	cancelParent()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if !queue.WaitProgress(ctx, observed) {
		cancel()
		t.Fatal("parent cancellation did not report queue progress")
	}
	cancel()
	if err := queue.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error = %v", err)
	}
	if queue.Backlog() != 0 || queue.Active() != 0 {
		t.Fatalf("canceled queue leaked resources: backlog=%d active=%d", queue.Backlog(), queue.Active())
	}
}

func TestProbeRefreshQueuePanicReleasesScheduledKey(t *testing.T) {
	queue := NewProbeRefreshQueue(context.Background(), ProbeRefreshOptions{WorkerCount: 1})
	defer queue.Close()
	observed := queue.Revision()
	if err := queue.Schedule(context.Background(), "main", func(context.Context) error { panic("synthetic") }); err != nil {
		t.Fatal(err)
	}
	if !queue.WaitProgress(context.Background(), observed) {
		t.Fatal("panic completion did not notify progress")
	}
	if err := queue.Schedule(context.Background(), "main", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("panic left key reserved: %v", err)
	}
}

func TestAvailabilitySchedulesNearExpiryRefreshWithoutBlocking(t *testing.T) {
	account := accountentity.Account{ID: "main", Enabled: true}
	usage := fakeUsage{byHome: map[string]quotamodel.Usage{"/managed/main": {PlanType: "plus"}}}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, usage)
	now := time.Unix(100, 0)
	status.now = func() time.Time { return now }
	if _, err := status.Availability(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	now = now.Add(usageCacheFreshness - probeRefreshLead/2)
	observed := status.ProbeRefreshRevision()
	if _, err := status.AvailabilityForRoute(context.Background(), account, quotamodel.Selection{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !status.WaitProbeRefresh(ctx, observed) {
		t.Fatal("near-expiry cache did not schedule a refresh")
	}
	if err := status.ShutdownProbeRefresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type probeRefreshTimeoutError struct{}

func (probeRefreshTimeoutError) Error() string   { return "synthetic timeout" }
func (probeRefreshTimeoutError) Timeout() bool   { return true }
func (probeRefreshTimeoutError) Temporary() bool { return true }
