package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type admissionGateway struct {
	entered chan struct{}
	release chan struct{}
	active  atomic.Int32
	max     atomic.Int32
	calls   atomic.Int32
}

func (gateway *admissionGateway) Execute(ctx context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	gateway.calls.Add(1)
	active := gateway.active.Add(1)
	for {
		current := gateway.max.Load()
		if active <= current || gateway.max.CompareAndSwap(current, active) {
			break
		}
	}
	select {
	case gateway.entered <- struct{}{}:
	case <-ctx.Done():
		gateway.active.Add(-1)
		return nil, ctx.Err()
	}
	select {
	case <-gateway.release:
	case <-ctx.Done():
		gateway.active.Add(-1)
		return nil, ctx.Err()
	}
	gateway.active.Add(-1)
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("{}")),
	}, nil
}

func newAdmissionParityProxy(t *testing.T, gateway *admissionGateway, limit int) *httptest.Server {
	t.Helper()
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "a", Home: "/synthetic/a", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, ActiveRequestLimit: limit})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy.server.Handler)
	t.Cleanup(server.Close)
	return server
}

func TestActiveRequestBackpressureSerializesThirtyTwoCallersWithoutRejection(t *testing.T) {
	const callers = 32
	gateway := &admissionGateway{
		entered: make(chan struct{}, callers),
		release: make(chan struct{}),
	}
	server := newAdmissionParityProxy(t, gateway, 1)
	client := server.Client()

	start := make(chan struct{})
	results := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for range callers {
		go func() {
			defer workers.Done()
			<-start
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/backend-api/codex/responses", strings.NewReader("{}"))
			if err != nil {
				results <- err
				return
			}
			response, err := client.Do(request)
			if err != nil {
				results <- err
				return
			}
			_, readErr := io.Copy(io.Discard, response.Body)
			closeErr := response.Body.Close()
			if response.StatusCode != http.StatusOK {
				results <- &unexpectedAdmissionStatus{status: response.StatusCode}
				return
			}
			if readErr != nil {
				results <- readErr
				return
			}
			results <- closeErr
		}()
	}
	close(start)

	for index := 0; index < callers; index++ {
		select {
		case <-gateway.entered:
		case <-time.After(5 * time.Second):
			t.Fatalf("caller %d never reached the single active slot", index+1)
		}
		if active := gateway.active.Load(); active != 1 {
			t.Fatalf("active upstream requests = %d, want 1", active)
		}
		select {
		case <-gateway.entered:
			t.Fatalf("more than one request entered upstream while limit=1")
		default:
		}
		gateway.release <- struct{}{}
	}
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if gateway.calls.Load() != callers || gateway.max.Load() != 1 {
		t.Fatalf("gateway calls/max active = %d/%d, want %d/1", gateway.calls.Load(), gateway.max.Load(), callers)
	}
}

func TestActiveRequestBackpressureHonorsCancellationWhileWaiting(t *testing.T) {
	gateway := &admissionGateway{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	server := newAdmissionParityProxy(t, gateway, 1)
	client := server.Client()

	firstDone := make(chan error, 1)
	go func() {
		response, err := client.Post(server.URL+"/backend-api/codex/responses", "application/json", strings.NewReader("{}"))
		if err == nil {
			_ = response.Body.Close()
		}
		firstDone <- err
	}()
	select {
	case <-gateway.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not occupy active slot")
	}

	ctx, cancel := context.WithCancel(context.Background())
	second, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/backend-api/codex/responses", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	secondDone := make(chan error, 1)
	go func() {
		response, requestErr := client.Do(second)
		if requestErr == nil {
			_ = response.Body.Close()
		}
		secondDone <- requestErr
	}()
	cancel()
	select {
	case err := <-secondDone:
		if err == nil {
			t.Fatal("canceled waiting request unexpectedly succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled waiting request did not stop")
	}
	if calls := gateway.calls.Load(); calls != 1 {
		t.Fatalf("canceled waiter dispatched upstream; calls=%d", calls)
	}

	gateway.release <- struct{}{}
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first request did not finish")
	}
}

type unexpectedAdmissionStatus struct{ status int }

func (err *unexpectedAdmissionStatus) Error() string {
	return http.StatusText(err.status)
}
