package routing

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type bindingsFake struct {
	values    []routingentity.Binding
	writes    int
	onAcquire func()
}

func (f *bindingsFake) Load(context.Context) ([]routingentity.Binding, error) {
	return append([]routingentity.Binding(nil), f.values...), nil
}
func (f *bindingsFake) AcquireConversation(context.Context) (func() error, error) {
	if f.onAcquire != nil {
		f.onAcquire()
	}
	return func() error { return nil }, nil
}
func (f *bindingsFake) Remove(_ context.Context, keys []string) error {
	removed := make(map[string]bool, len(keys))
	for _, key := range keys {
		removed[key] = true
	}
	values := f.values[:0]
	for _, binding := range f.values {
		if !removed[binding.Key] {
			values = append(values, binding)
		}
	}
	f.values = values
	return nil
}

func TestDurableOwnerBeyondCacheCapacity(t *testing.T) {
	now := time.Now()
	repository := &bindingsFake{}
	for i := 0; i <= affinityMaxValues; i++ {
		entry := affinityKeys{thread: fmt.Sprintf("thread-%d", i)}.entries()[0]
		entry.AccountID, entry.UpdatedUnix = "owner", now.Unix()-int64(i)
		repository.values = append(repository.values, entry)
	}
	store := newAffinityStore()
	store.repository = repository
	keys := affinityKeys{thread: fmt.Sprintf("thread-%d", affinityMaxValues)}
	owner, err := store.owner(context.Background(), keys, now)
	if err != nil || owner != "owner" {
		t.Fatalf("oldest durable owner = %q, %v", owner, err)
	}
	if len(store.values) > affinityMaxValues {
		t.Fatal("affinity cache exceeded its bound")
	}
}

func TestDurableOwnerRefreshesStaleCacheUnderConversationLock(t *testing.T) {
	keys := affinityKeys{session: "synthetic-session"}
	binding := keys.entries()[0]
	binding.AccountID = "account-a"
	repository := &bindingsFake{values: []routingentity.Binding{binding}}
	store := newAffinityStore()
	store.repository = repository
	now := time.Now()
	if owner, err := store.owner(context.Background(), keys, now); err != nil || owner != "account-a" {
		t.Fatalf("initial owner = %q, %v", owner, err)
	}
	binding.AccountID = "account-b"
	repository.onAcquire = func() { repository.values = []routingentity.Binding{binding} }
	release, err := repository.AcquireConversation(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	owner, err := store.refreshOwner(context.Background(), keys, now)
	if err != nil || owner != "account-b" {
		t.Fatalf("refreshed owner = %q, %v", owner, err)
	}
}

func (f *bindingsFake) Merge(_ context.Context, updates []routingentity.Binding) ([]routingentity.Binding, error) {
	f.writes++
	f.values = append(f.values, updates...)
	return f.values, nil
}
func TestDurableAffinityRecoversAfterExpiryAndDoesNotWritePerChunk(t *testing.T) {
	repository := &bindingsFake{}
	store := newAffinityStore()
	store.repository = repository
	now := time.Now()
	keys := affinityKeys{thread: "synthetic-thread", previous: "synthetic-response", turn: "synthetic-opaque-state"}
	account := strings.Repeat("a", 32)
	if err := store.remember(context.Background(), account, keys, now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := store.remember(context.Background(), account, keys, now); err != nil {
			t.Fatal(err)
		}
	}
	if repository.writes != 1 {
		t.Fatalf("writes per observation = %d", repository.writes)
	}
	store = newAffinityStore()
	store.repository = repository
	owner, err := store.owner(context.Background(), keys, now.Add(affinityTTL))
	if err != nil || owner != account {
		t.Fatalf("recovered owner = %q, %v", owner, err)
	}
	for _, binding := range repository.values {
		if strings.Contains(binding.Key, "synthetic") {
			t.Fatal("raw continuity metadata persisted")
		}
	}
}

func TestForgetRemovesCachedAndPersistedConversationOwnership(t *testing.T) {
	repository := &bindingsFake{}
	store := newAffinityStore()
	store.repository = repository
	keys := affinityKeys{session: "synthetic-session", previous: "synthetic-response"}
	if err := store.remember(context.Background(), strings.Repeat("a", 32), keys, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.forget(context.Background(), keys); err != nil {
		t.Fatal(err)
	}
	owner, err := store.owner(context.Background(), keys, time.Now())
	if err != nil || owner != "" || len(repository.values) != 0 {
		t.Fatalf("forgotten conversation owner = %q, persisted bindings = %d, error = %v", owner, len(repository.values), err)
	}
}

type countingGateway struct {
	mu     sync.Mutex
	owners []string
}

func (f *countingGateway) Execute(_ context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	f.mu.Lock()
	f.owners = append(f.owners, account.ID)
	f.mu.Unlock()
	return &proxymodel.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}
func TestConcurrentFirstCallsReserveOneConversationOwner(t *testing.T) {
	gateway := &countingGateway{}
	router, err := NewRouter(Config{Gateway: gateway, Accounts: func(context.Context) ([]proxymodel.Account, error) {
		return []proxymodel.Account{{ID: "one", Home: "one", Enabled: true}, {ID: "two", Home: "two", Enabled: true}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			exchange, err := router.Forward(context.Background(), proxymodel.Request{Header: http.Header{"Thread-Id": []string{"same-thread"}}})
			if err != nil {
				t.Error(err)
				return
			}
			exchange.Close()
		}()
	}
	group.Wait()
	if len(gateway.owners) != 2 || gateway.owners[0] != gateway.owners[1] {
		t.Fatalf("conversation split = %v", gateway.owners)
	}
	exchange, err := router.Forward(context.Background(), proxymodel.Request{Header: make(http.Header), Body: []byte(`{"previous_response_id":"unknown"}`)})
	if exchange != nil || err == nil {
		t.Fatal("unknown continuation was treated as fresh")
	}
}
