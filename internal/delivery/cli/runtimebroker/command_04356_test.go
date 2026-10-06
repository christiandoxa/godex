package runtimebroker

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type brokerCommandAccounts struct {
	home string
}

func (accounts brokerCommandAccounts) LaunchCandidates(context.Context, string) ([]accountentity.Account, error) {
	return []accountentity.Account{{ID: "account-a", Name: "main", Enabled: true}}, nil
}
func (accounts brokerCommandAccounts) SelectForLaunch(context.Context, string) (accountentity.Account, error) {
	return accountentity.Account{ID: "account-a", Name: "main", Enabled: true}, nil
}
func (accounts brokerCommandAccounts) List(context.Context) ([]accountentity.Account, error) {
	return []accountentity.Account{{ID: "account-a", Name: "main", Enabled: true}}, nil
}
func (accounts brokerCommandAccounts) CodexHome(string) string { return accounts.home }

type brokerCommandProcess struct{}

func (brokerCommandProcess) Run(context.Context, string, []string) error { return nil }

type brokerCommandProfiles struct {
	home string
}

func (profiles brokerCommandProfiles) ResolveLaunch(context.Context, string) (profilemodel.LaunchTarget, error) {
	return profilemodel.LaunchTarget{
		Name: "main", CodexHome: profiles.home, AccountID: "account-a", Provider: "openai",
	}, nil
}

type brokerCommandProxy struct {
	mu                 sync.Mutex
	started            chan struct{}
	promoted           chan struct{}
	closed             bool
	persistenceEnabled bool
	role               string
	logs               []string
	broker             *proxymodel.BrokerConfig
}

func newBrokerCommandProxy() *brokerCommandProxy {
	return &brokerCommandProxy{
		started: make(chan struct{}), promoted: make(chan struct{}),
	}
}
func (proxy *brokerCommandProxy) Start() error {
	select {
	case <-proxy.started:
	default:
		close(proxy.started)
	}
	return nil
}
func (*brokerCommandProxy) Endpoint() string { return "http://127.0.0.1:4321" }
func (proxy *brokerCommandProxy) Close(context.Context) error {
	proxy.mu.Lock()
	proxy.closed = true
	proxy.mu.Unlock()
	return nil
}
func (*brokerCommandProxy) ActiveRequests() int { return 0 }
func (proxy *brokerCommandProxy) SetPersistenceEnabled(enabled bool) {
	proxy.mu.Lock()
	proxy.persistenceEnabled = enabled
	proxy.mu.Unlock()
}
func (proxy *brokerCommandProxy) SetBrokerPersistenceRole(role string) {
	proxy.mu.Lock()
	proxy.role = role
	if proxy.broker != nil {
		proxy.broker.PersistenceRole = role
	}
	if role == "owner" {
		select {
		case <-proxy.promoted:
		default:
			close(proxy.promoted)
		}
	}
	proxy.mu.Unlock()
}
func (proxy *brokerCommandProxy) RecordBrokerLog(line string) {
	proxy.mu.Lock()
	proxy.logs = append(proxy.logs, line)
	proxy.mu.Unlock()
}
func (proxy *brokerCommandProxy) snapshot() (bool, string, []string) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	return proxy.persistenceEnabled, proxy.role, append([]string(nil), proxy.logs...)
}

func TestProdex04356RuntimeBrokerOwnerRoleGatesPersistenceAndPublishesBirthIdentity(t *testing.T) {
	command, store, proxy := testBrokerCommand(t)
	command.poll = time.Millisecond
	command.leaseScan = time.Millisecond
	command.grace = time.Second
	command.startupGrace = time.Nanosecond
	command.now = steppedBrokerClock(1000)

	if err := command.Run(t.Context(), strings.NewReader(testBrokerBootstrap("owner-instance"))); err != nil {
		t.Fatal(err)
	}
	enabled, role, logs := proxy.snapshot()
	if !enabled || role != "owner" {
		t.Fatalf("owner persistence state = enabled:%t role:%q logs:%#v", enabled, role, logs)
	}
	if !proxy.closed {
		t.Fatal("owner broker proxy was not closed")
	}
	if processBirthIdentity(0) != "" {
		t.Fatal("zero pid unexpectedly has process birth identity")
	}
	if (runtime.GOOS == "linux" || runtime.GOOS == "windows") &&
		processBirthIdentity(uint32(os.Getpid())) == "" {
		t.Fatal("current process birth identity is unavailable on supported platform")
	}
	if _, found, err := store.LoadRegistry(t.Context(), "broker-key"); err != nil || found {
		t.Fatalf("broker registry not cleaned after exit: found=%t err=%v", found, err)
	}
}

func TestProdex04356RuntimeBrokerFollowerPromotesWhenOwnerLockReleases(t *testing.T) {
	command, store, proxy := testBrokerCommand(t)
	command.poll = 5 * time.Millisecond
	command.leaseScan = 5 * time.Millisecond
	command.grace = time.Hour
	command.startupGrace = time.Hour

	heldRelease, owner, err := store.TryAcquireOwner()
	if err != nil || !owner {
		t.Fatalf("hold owner lock = owner:%t err:%v", owner, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- command.Run(ctx, strings.NewReader(testBrokerBootstrap("follower-instance")))
	}()

	select {
	case <-proxy.started:
	case <-time.After(2 * time.Second):
		t.Fatal("follower broker did not start")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		enabled, role, logs := proxy.snapshot()
		if !enabled && role == "follower" {
			found := false
			for _, line := range logs {
				found = found || strings.Contains(line, "runtime_broker_persistence_follower")
			}
			if !found {
				t.Fatalf("follower role missing log: %#v", logs)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("follower role not observed: enabled=%t role=%q logs=%#v", enabled, role, logs)
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := heldRelease(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-proxy.promoted:
	case <-time.After(2 * time.Second):
		t.Fatal("follower broker did not promote after owner lock release")
	}
	enabled, role, logs := proxy.snapshot()
	if !enabled || role != "owner" {
		t.Fatalf("promoted state = enabled:%t role:%q logs:%#v", enabled, role, logs)
	}
	promotedLog := false
	for _, line := range logs {
		promotedLog = promotedLog || strings.Contains(line, "runtime_broker_persistence_promoted role=owner")
	}
	if !promotedLog {
		t.Fatalf("promotion log missing: %#v", logs)
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("follower broker cancel = %v", err)
	}
}

func TestProdex04356RuntimeBrokerStartupGraceMatchesReadyTimeoutContract(t *testing.T) {
	if got := brokerStartupGrace(15*time.Second, 5*time.Second); got != 16*time.Second {
		t.Fatalf("startup grace = %s, want 16s", got)
	}
	if got := brokerStartupGrace(2*time.Second, 5*time.Second); got != 5*time.Second {
		t.Fatalf("startup grace floor = %s, want 5s", got)
	}
}

func testBrokerCommand(t *testing.T) (*Command, *Store, *brokerCommandProxy) {
	t.Helper()
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	proxy := newBrokerCommandProxy()
	accounts := brokerCommandAccounts{home: root + "/profile"}
	if err := os.MkdirAll(accounts.home, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := runtimeusecase.NewRunner(accounts, brokerCommandProcess{}, func(config proxymodel.Config) (runtimeusecase.Proxy, error) {
		proxy.mu.Lock()
		proxy.broker = config.Broker
		proxy.mu.Unlock()
		return proxy, nil
	})
	runner.SetUpstreamURL("http://127.0.0.1:9/backend-api")
	command := NewCommand(runner, brokerCommandProfiles{home: accounts.home}, store, nil, "0.435.6")
	return command, store, proxy
}

func steppedBrokerClock(start int64) func() time.Time {
	var mu sync.Mutex
	value := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		current := time.Unix(value, 0)
		value++
		return current
	}
}

func testBrokerBootstrap(instance string) string {
	return `{
		"version":1,
		"current_profile":"main",
		"upstream_base_url":"http://127.0.0.1:9/backend-api",
		"include_code_review":false,
		"upstream_no_proxy":true,
		"smart_context_enabled":false,
		"model_context_window_tokens":null,
		"broker_key":"broker-key",
		"instance_id":"` + instance + `",
		"admin_token":"synthetic-admin"
	}`
}
