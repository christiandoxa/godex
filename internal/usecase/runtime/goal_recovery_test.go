package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

type cancellationMonitorProcess struct {
	started chan struct{}
	err     error
}

func (process *cancellationMonitorProcess) Run(ctx context.Context, _ string, _ []string) error {
	close(process.started)
	if process.err != nil {
		return process.err
	}
	<-ctx.Done()
	return ctx.Err()
}

func cancellationMonitorRunner(process codexProcess) *Runner {
	accounts := &fakeLaunchAccounts{
		accounts: []accountentity.Account{{ID: "synthetic", Enabled: true}},
		homes:    map[string]string{"synthetic": "/synthetic/codex"},
	}
	return NewRunner(accounts, process, nil)
}

type nativeRecoveryMonitorProcess struct {
	started   chan struct{}
	recovered chan struct{}
	once      sync.Once
	mu        sync.Mutex
	calls     int
}

func (process *nativeRecoveryMonitorProcess) Run(ctx context.Context, _ string, _ []string) error {
	close(process.started)
	<-ctx.Done()
	return ctx.Err()
}

func (process *nativeRecoveryMonitorProcess) RecoverNativeSession(context.Context, string, bool) bool {
	process.mu.Lock()
	process.calls++
	process.mu.Unlock()
	process.once.Do(func() { close(process.recovered) })
	return true
}

type goalRecoveryProfilesFake struct {
	homes   []sessionmodel.ProfileHome
	targets map[string]profilemodel.LaunchTarget
}

func (profiles goalRecoveryProfilesFake) SessionProfiles(context.Context) ([]sessionmodel.ProfileHome, error) {
	return profiles.homes, nil
}

func (profiles goalRecoveryProfilesFake) ResolveLaunch(_ context.Context, name string) (profilemodel.LaunchTarget, error) {
	return profiles.targets[name], nil
}

func TestRunWithGoalRecoveryMonitorStopsForFreshLimitAndPreservesOtherExit(t *testing.T) {
	trigger := make(chan struct{})
	armed := make(chan struct{})
	var arm sync.Once
	process := &cancellationMonitorProcess{started: make(chan struct{})}
	runner := cancellationMonitorRunner(process)
	go func() {
		<-process.started
		<-armed
		close(trigger)
	}()
	err, triggered := runner.RunWithGoalRecoveryMonitor(
		t.Context(), "", nil, RuntimeLaunchOptions{}, func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
			select {
			case <-trigger:
				return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "usage_limited", UpdatedAt: 1}, true
			default:
				arm.Do(func() { close(armed) })
				return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "active"}, true
			}
		},
	)
	if !triggered || !errors.Is(err, context.Canceled) {
		t.Fatalf("triggered=%t err=%v", triggered, err)
	}

	want := errors.New("synthetic child exit")
	process = &cancellationMonitorProcess{started: make(chan struct{}), err: want}
	runner = cancellationMonitorRunner(process)
	err, triggered = runner.RunWithGoalRecoveryMonitor(
		t.Context(), "", nil, RuntimeLaunchOptions{}, func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{}, false
		},
	)
	if triggered || !errors.Is(err, want) {
		t.Fatalf("ordinary child exit was changed: triggered=%t err=%v", triggered, err)
	}
}

func TestRunWithGoalRecoveryMonitorParentCancellationStopsWatcher(t *testing.T) {
	process := &cancellationMonitorProcess{started: make(chan struct{})}
	runner := cancellationMonitorRunner(process)
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-process.started
		cancel()
	}()
	err, triggered := runner.RunWithGoalRecoveryMonitor(
		ctx, "", nil, RuntimeLaunchOptions{}, func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "active"}, true
		},
	)
	if triggered || !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation changed: triggered=%t err=%v", triggered, err)
	}
}

func TestRunWithGoalRecoveryMonitorKeepsOwnedFrontendAndDoesNotReplayNativeRecovery(t *testing.T) {
	process := &nativeRecoveryMonitorProcess{
		started: make(chan struct{}), recovered: make(chan struct{}),
	}
	runner := cancellationMonitorRunner(process)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		<-process.recovered
		cancel()
	}()
	reads := 0
	err, triggered := runner.RunWithGoalRecoveryMonitor(
		ctx, "", nil, RuntimeLaunchOptions{}, func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
			reads++
			if reads == 1 {
				return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "active"}, true
			}
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "usage_limited"}, true
		},
	)
	process.mu.Lock()
	calls := process.calls
	process.mu.Unlock()
	if triggered || !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("native recovery calls=%d triggered=%t err=%v", calls, triggered, err)
	}
}

type retryingNativeRecoveryMonitorProcess struct {
	started chan struct{}
	calls   chan struct{}
}

func (process *retryingNativeRecoveryMonitorProcess) Run(ctx context.Context, _ string, _ []string) error {
	close(process.started)
	<-ctx.Done()
	return ctx.Err()
}

func (process *retryingNativeRecoveryMonitorProcess) RecoverNativeSession(context.Context, string, bool) bool {
	process.calls <- struct{}{}
	return true
}

func TestRunWithGoalRecoveryMonitorRetriesOwnedNativeRecoveryUntilItSettles(t *testing.T) {
	process := &retryingNativeRecoveryMonitorProcess{
		started: make(chan struct{}), calls: make(chan struct{}, 4),
	}
	runner := cancellationMonitorRunner(process)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	observed := make(chan int, 1)
	go func() {
		<-process.started
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		received := 0
		for received < 2 {
			select {
			case <-process.calls:
				received++
			case <-timer.C:
				observed <- received
				cancel()
				return
			}
		}
		observed <- received
		cancel()
	}()
	reads := 0
	err, triggered := runner.RunWithGoalRecoveryMonitor(ctx, "", nil, RuntimeLaunchOptions{}, func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
		reads++
		if reads == 1 {
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "active"}, true
		}
		return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "usage_limited"}, true
	})
	if triggered || !errors.Is(err, context.Canceled) {
		t.Fatalf("native recovery retry changed launch: triggered=%t err=%v", triggered, err)
	}
	if received := <-observed; received < 2 {
		t.Fatalf("owned native recovery calls=%d, want at least 2", received)
	}
}

func TestWaitForGoalRecoveryProfileWaitsForQuotaReadyBackup(t *testing.T) {
	accounts := &fakeLaunchAccounts{accounts: []accountentity.Account{
		{ID: "a", Name: "primary", Enabled: true},
		{ID: "b", Name: "backup", Enabled: true},
	}, homes: map[string]string{"a": "/profiles/a", "b": "/profiles/b"}}
	quota := &fakeQuotaPreflight{ready: map[string]bool{"a": true}, errs: map[string]error{}}
	runner := NewRunner(accounts, &fakeProcess{}, nil)
	runner.SetQuotaPreflight(quota)
	profiles := goalRecoveryProfilesFake{
		homes: []sessionmodel.ProfileHome{
			{Name: "primary", AccountID: "a", Provider: "openai", Enabled: true},
			{Name: "backup", AccountID: "b", Provider: "openai", Enabled: true},
		},
		targets: map[string]profilemodel.LaunchTarget{
			"primary": {Name: "primary", AccountID: "a", Provider: "openai", Auth: "chatgpt"},
			"backup":  {Name: "backup", AccountID: "b", Provider: "openai", Auth: "chatgpt"},
		},
	}
	waits := 0
	target, ok := runner.WaitForGoalRecoveryProfile(
		t.Context(), "session",
		func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "usage_limited"}, true
		}, profiles, map[string]bool{"a": true}, RuntimeLaunchOptions{},
		func(context.Context) bool {
			waits++
			quota.ready["b"] = true
			return true
		},
	)
	if !ok || target.AccountID != "b" || waits != 1 {
		t.Fatalf("target=%#v ok=%t waits=%d", target, ok, waits)
	}
}

func TestWaitForGoalRecoveryProfileAllowsExhaustedBackupWithAutoRedeem(t *testing.T) {
	accounts := &fakeLaunchAccounts{accounts: []accountentity.Account{
		{ID: "a", Name: "primary", Enabled: true},
		{ID: "b", Name: "backup", Enabled: true},
	}, homes: map[string]string{"a": "/profiles/a", "b": "/profiles/b"}}
	runner := NewRunner(accounts, &fakeProcess{}, nil)
	runner.SetQuotaPreflight(&fakeQuotaPreflight{ready: map[string]bool{}, errs: map[string]error{}})
	profiles := goalRecoveryProfilesFake{
		homes: []sessionmodel.ProfileHome{
			{Name: "primary", AccountID: "a", Provider: "openai", Enabled: true},
			{Name: "backup", AccountID: "b", Provider: "openai", Enabled: true},
		},
		targets: map[string]profilemodel.LaunchTarget{
			"primary": {Name: "primary", AccountID: "a", Provider: "openai", Auth: "chatgpt"},
			"backup":  {Name: "backup", AccountID: "b", Provider: "openai", Auth: "chatgpt"},
		},
	}
	autoRedeem := true
	target, ok := runner.WaitForGoalRecoveryProfile(
		t.Context(), "session",
		func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "usage_limited"}, true
		}, profiles, map[string]bool{"a": true}, RuntimeLaunchOptions{AutoRedeem: &autoRedeem},
		func(context.Context) bool { t.Fatal("auto-redeem waited for quota readiness"); return false },
	)
	if !ok || target.AccountID != "b" {
		t.Fatalf("target=%#v ok=%t", target, ok)
	}
}
