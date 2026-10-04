package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/codex"
	"github.com/christiandoxa/godex/internal/gateway/openai"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingrepo "github.com/christiandoxa/godex/internal/repository/routing"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestRotatedConversationSurvivesProxyRestart(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	accounts[0].ID = strings.Repeat("a", 32)
	accounts[1].ID = strings.Repeat("b", 32)
	home := t.TempDir()
	bindings := routingrepo.NewStore(home)
	calls := 0
	upstream := httptest.NewServer(durableRotationHandler(t, &calls))
	defer upstream.Close()

	first := launchDurableProxy(t, upstream.URL, bindings, accounts)
	closeProxyResponse(t, doProxyJSON(t, first.URL+"/responses", "{}", map[string]string{"thread-id": "native-thread"}))
	first.Close()
	assertDurableOwner(t, bindings, "native-thread", accounts[1].ID)

	second := launchDurableProxy(t, upstream.URL, bindings, accounts)
	response := doProxyJSON(t, second.URL+"/responses", "{\\\"previous_response_id\\\":\\\"response-2\\\"}", map[string]string{"thread-id": "native-thread"})
	closeProxyResponse(t, response)
	if response.StatusCode != http.StatusOK || calls != 3 {
		t.Fatalf("restart status/calls = %d/%d", response.StatusCode, calls)
	}
	assertRoutingSnapshotHasNoRawData(t, home, "native-thread", "response-2", "token-a", "token-b")
}

func durableRotationHandler(t *testing.T, calls *int) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		*calls = *calls + 1
		if *calls == 1 {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(writer, `{"error":{"code":"insufficient_quota"}}`)
			return
		}
		if request.Header.Get("ChatGPT-Account-Id") != "workspace-B" {
			t.Error("continuation changed its rotated owner")
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, "{\\\"id\\\":\\\"response-%d\\\"}", *calls)
	})
}

func launchDurableProxy(t *testing.T, upstreamURL string, bindings *routingrepo.Store, accounts []proxymodel.Account) *httptest.Server {
	t.Helper()
	transport, err := openai.NewTransport(upstreamURL, nil, codex.NewCodexProcess("", codex.Terminal{}))
	if err != nil {
		t.Fatal(err)
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:  transport,
		Bindings: bindings,
		Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	t.Cleanup(func() {
		server.Close()
		router.Close()
	})
	return server
}

func closeProxyResponse(t *testing.T, response *http.Response) {
	t.Helper()
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertDurableOwner(t *testing.T, bindings *routingrepo.Store, thread, want string) {
	t.Helper()
	owner, err := routingusecase.SessionOwner(context.Background(), bindings, thread)
	if err != nil || owner != want {
		t.Fatalf("durable upstream owner = %q, want %q: %v", owner, want, err)
	}
}

func assertRoutingSnapshotHasNoRawData(t *testing.T, home string, values ...string) {
	t.Helper()
	snapshot, err := os.ReadFile(filepath.Join(home, "routing.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range values {
		if strings.Contains(string(snapshot), raw) {
			t.Fatalf("raw continuity or credential data persisted: %q", raw)
		}
	}
}

type observedBindingStore struct {
	*routingrepo.Store
	locking chan struct{}
}

func (store observedBindingStore) AcquireConversation(ctx context.Context) (func() error, error) {
	store.locking <- struct{}{}
	return store.Store.AcquireConversation(ctx)
}

type firstOwnerGateway struct {
	mu               sync.Mutex
	owners           []string
	started, proceed chan struct{}
}

func (gateway *firstOwnerGateway) Execute(ctx context.Context, _ proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.owners = append(gateway.owners, account.ID)
	first := len(gateway.owners) == 1
	gateway.mu.Unlock()
	if first {
		close(gateway.started)
		select {
		case <-gateway.proceed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &proxymodel.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}

func TestIndependentRoutersSerializeFirstConversationOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	store := observedBindingStore{routingrepo.NewStore(t.TempDir()), make(chan struct{}, 2)}
	gateway := &firstOwnerGateway{started: make(chan struct{}), proceed: make(chan struct{})}
	var once sync.Once
	unblock := func() { once.Do(func() { close(gateway.proceed) }) }
	defer unblock()
	accounts := []proxymodel.Account{{ID: strings.Repeat("a", 32), Home: "synthetic-a", Enabled: true}, {ID: strings.Repeat("b", 32), Home: "synthetic-b", Enabled: true}}
	results := make(chan error, 2)
	start := func(preferred string) {
		router, err := routingusecase.NewRouter(routingusecase.Config{Gateway: gateway, Bindings: store, PreferredAccount: preferred, Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }})
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			exchange, err := router.Forward(ctx, proxymodel.Request{Header: http.Header{"Thread-Id": []string{"shared-thread"}}})
			if err == nil {
				err = exchange.Close()
			}
			results <- err
		}()
	}
	start(accounts[0].ID)
	select {
	case <-gateway.started:
	case <-ctx.Done():
		t.Fatal("first upstream request did not start")
	}
	<-store.locking
	start(accounts[1].ID)
	select {
	case <-store.locking:
	case <-ctx.Done():
		t.Fatal("second router did not acquire the durable conversation guard")
	}
	unblock()
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("routers did not complete")
		}
	}
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.owners) != 2 || gateway.owners[0] != gateway.owners[1] {
		t.Fatalf("conversation split: %v", gateway.owners)
	}
}
