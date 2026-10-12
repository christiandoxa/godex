package runtime

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	sessionrepo "github.com/christiandoxa/godex/internal/repository/session"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
	sessionusecase "github.com/christiandoxa/godex/internal/usecase/session"
)

const freshSuperRecoverySession04377 = "019c9e3d-45a0-7ad0-a6ee-b194ac2d44f9"

type freshSuperNativeProcess04377 struct {
	started     chan struct{}
	recovered   chan bool
	startOnce   sync.Once
	recoverOnce sync.Once
}

func (process *freshSuperNativeProcess04377) Run(context.Context, string, []string) error {
	return errors.New("fresh Super unexpectedly bypassed its session app-server")
}

func (*freshSuperNativeProcess04377) CheckProxySupport(context.Context) error { return nil }

func (*freshSuperNativeProcess04377) RunThroughProxy(context.Context, string, string, []string) error {
	return errors.New("fresh Super unexpectedly bypassed its session app-server")
}

func (process *freshSuperNativeProcess04377) RunThroughProxyWithSessionServerAndRecovery(
	ctx context.Context, _ string, _ string, arguments []string, _ string,
	_ func(context.Context, string) error,
) error {
	if marker := markerPathFromArgs04361(arguments); marker != "" {
		if err := os.WriteFile(marker, []byte(freshSuperRecoverySession04377+"\n"), 0o600); err != nil {
			return err
		}
	}
	process.startOnce.Do(func() { close(process.started) })
	<-ctx.Done()
	return ctx.Err()
}

func (process *freshSuperNativeProcess04377) RecoverNativeSession(
	context.Context, string, bool,
) runtimemodel.NativeRecoveryOutcome {
	process.recoverOnce.Do(func() { close(process.recovered) })
	return runtimemodel.NativeRecoveryAccepted
}

type freshSuperRecoveryStateReader04377 struct {
	mu    sync.Mutex
	reads int
}

func (reader *freshSuperRecoveryStateReader04377) ReadGoalRecoveryState(
	context.Context, string,
) (runtimemodel.GoalRecoveryState, bool) {
	reader.mu.Lock()
	reader.reads++
	reads := reader.reads
	reader.mu.Unlock()
	if reads == 1 {
		return runtimemodel.GoalRecoveryState{
			SessionID: freshSuperRecoverySession04377, Status: "active",
		}, true
	}
	return runtimemodel.GoalRecoveryState{
		SessionID: freshSuperRecoverySession04377, Status: "usage_limited", UpdatedAt: 1,
	}, true
}

func freshSuperRecoveryFixture04377(t *testing.T) (
	context.Context, context.CancelFunc, *freshSuperNativeProcess04377,
	*runtimeusecase.Runner, *sessionusecase.Catalog, *fakeLocalLaunchProfiles,
) {
	t.Helper()
	root := t.TempDir()
	home := root + "/home"
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home+"/config.toml", []byte("model = \"synthetic\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	accounts := &runPolicyAccounts{
		values: []accountentity.Account{{ID: "a", Name: "primary", Enabled: true}},
		homes:  map[string]string{"a": home},
	}
	process := &freshSuperNativeProcess04377{
		started: make(chan struct{}), recovered: make(chan bool, 1),
	}
	reader := &freshSuperRecoveryStateReader04377{}
	runner := runtimeusecase.NewRunner(accounts, process, func(proxymodel.Config) (runtimeusecase.Proxy, error) {
		return &gatewayTestProxy{}, nil
	}, reader)
	runner.SetSharedCodexHome(root + "/shared")
	runner.SetManagedProfilesRoot(root + "/profiles")
	profiles := &fakeLocalLaunchProfiles{target: profilemodel.LaunchTarget{
		Name: "primary", AccountID: "a", CodexHome: home, Provider: "openai", Auth: "chatgpt",
	}, active: true}
	sessions := sessionusecase.NewCatalog(accounts, sessionrepo.NewReader(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	return ctx, cancel, process, runner, sessions, profiles
}

func TestProdex04377FreshSuperUsesOwnedNativeRecoveryMonitor(t *testing.T) {
	ctx, cancel, process, runner, sessions, profiles := freshSuperRecoveryFixture04377(t)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- runParsedWithOptionsWithRecovery(ctx, runner, sessions, profiles, "a", []string{"--model", "synthetic"}, runtimeusecase.RuntimeLaunchOptions{
			AllowAutoRotate: func() *bool { value := true; return &value }(),
			SuperOverlay:    true,
		})
	}()
	select {
	case <-process.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fresh Super child did not start")
	}
	select {
	case <-process.recovered:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("fresh Super did not invoke owned native recovery")
	}
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("fresh Super monitor error = %v", err)
	}
}

func TestProdex04377FreshSuperNoAutoRotateSkipsNativeRecoveryMonitor(t *testing.T) {
	ctx, cancel, process, runner, sessions, profiles := freshSuperRecoveryFixture04377(t)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- runParsedWithOptionsWithRecovery(ctx, runner, sessions, profiles, "a", []string{"--model", "synthetic"}, runtimeusecase.RuntimeLaunchOptions{
			AllowAutoRotate: func() *bool { value := false; return &value }(),
			SuperOverlay:    true,
		})
	}()
	select {
	case <-process.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fresh Super child did not start")
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("fresh Super no-auto-rotate error = %v", err)
	}
	select {
	case <-process.recovered:
		t.Fatal("no-auto-rotate fresh Super invoked native recovery")
	default:
	}
}
