package quota

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProbeRefreshGateBackpressuresAtCapacityAndHonorsCancellation(t *testing.T) {
	var gate probeRefreshGate
	for range quotaProbeCapacity {
		if err := gate.acquire(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case gate.slots <- struct{}{}:
		t.Fatal("full probe gate accepted an extra request")
	default:
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("full gate acquire error = %v, want context.Canceled", err)
	}
	for range quotaProbeCapacity {
		gate.release()
	}
	if err := gate.acquire(context.Background()); err != nil {
		t.Fatalf("released gate acquire error = %v", err)
	}
	gate.release()
}

func TestProviderErrorReleasesProbeCapacity(t *testing.T) {
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	usage := &errorUsage{err: errors.New("synthetic provider failure")}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, usage)
	status.now = func() time.Time { return time.Unix(100, 0) }

	for attempt := 0; attempt < quotaProbeCapacity+1; attempt++ {
		_, err := status.Availability(context.Background(), account)
		if !errors.Is(err, usage.err) {
			t.Fatalf("attempt %d error = %v, want provider error", attempt, err)
		}
	}
	if usage.calls != quotaProbeCapacity+1 {
		t.Fatalf("provider calls = %d, want %d", usage.calls, quotaProbeCapacity+1)
	}
}

func TestStalePersistedProbeIsRejectedAfterItsReset(t *testing.T) {
	now := time.Unix(10_000, 0)
	account := accountentity.Account{ID: "stale", Name: "stale", Enabled: true}
	reset := now.Add(-time.Second).Unix()
	store := &memoryUsageSnapshotStore{values: map[string]quotamodel.UsageSnapshot{
		account.ID: {
			CheckedAt:      now.Add(-time.Minute).Unix(),
			FiveHourStatus: quotamodel.WindowExhausted, FiveHourRemainingPercent: 0, FiveHourResetAt: reset,
			WeeklyStatus: quotamodel.WindowReady, WeeklyRemainingPercent: 80, WeeklyResetAt: now.Add(time.Hour).Unix(),
		},
	}}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, failingUsage{})
	status.now = func() time.Time { return now }
	status.SetUsageSnapshotStore(store)

	_, err := status.Availability(context.Background(), account)
	if err == nil {
		t.Fatal("expired persisted probe unexpectedly masked provider failure")
	}
}

func TestStatusRunPreservesCandidateOrderAndStopsOnCancellation(t *testing.T) {
	first := accountentity.Account{ID: "first", Name: "first", Enabled: true}
	second := accountentity.Account{ID: "second", Name: "second", Enabled: true}
	ctx, cancel := context.WithCancel(context.Background())
	usage := &orderedUsage{cancelAfter: 1, cancel: cancel}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{first, second}, current: first}, usage)

	reports, err := status.Run(ctx, Options{All: true})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context.Canceled", err)
	}
	if len(reports) != 0 {
		t.Fatalf("canceled run returned %d reports", len(reports))
	}
	if got, want := usage.homes, []string{"/managed/first"}; !equalStrings(got, want) {
		t.Fatalf("probe order = %#v, want %#v", got, want)
	}
}

type errorUsage struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (usage *errorUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	usage.mu.Lock()
	usage.calls++
	usage.mu.Unlock()
	return quotamodel.Usage{}, usage.err
}

type orderedUsage struct {
	mu          sync.Mutex
	homes       []string
	cancelAfter int
	cancel      context.CancelFunc
}

func (usage *orderedUsage) Fetch(_ context.Context, home string) (quotamodel.Usage, error) {
	usage.mu.Lock()
	usage.homes = append(usage.homes, home)
	if len(usage.homes) == usage.cancelAfter && usage.cancel != nil {
		usage.cancel()
	}
	usage.mu.Unlock()
	return quotamodel.Usage{}, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
