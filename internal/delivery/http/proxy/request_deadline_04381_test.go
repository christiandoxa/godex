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

func oneLaneAdmissionLimits04381() admissionLimits {
	return admissionLimits{global: 1, lane: [admissionLaneCount]int{1, 1, 1, 1}}
}

func TestProdex04381AdmissionExpiryDoesNotDispatchAndFreshRequestSucceeds(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		} else {
			writer.WriteHeader(http.StatusOK)
		}
	})
	handler, ok := newActiveRequestHandlerWithLimits(next, oneLaneAdmissionLimits04381()).(*activeRequestHandler)
	if !ok {
		t.Fatal("admission handler type changed")
	}
	handler.requestBudget = 35 * time.Millisecond

	firstDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/backend-api/godex/responses", nil))
		close(firstDone)
	}()
	<-entered

	expired := httptest.NewRecorder()
	handler.ServeHTTP(expired, httptest.NewRequest(http.MethodPost, "/backend-api/godex/responses", nil))
	if expired.Code != http.StatusGatewayTimeout || !strings.Contains(expired.Body.String(), `"request_timeout"`) {
		t.Fatalf("expired admission = %d/%q, want local 504 request_timeout", expired.Code, expired.Body.String())
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expired admission dispatched next handler %d times", got)
	}

	close(release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first request did not release admission")
	}
	fresh := httptest.NewRecorder()
	handler.ServeHTTP(fresh, httptest.NewRequest(http.MethodPost, "/backend-api/godex/responses", nil))
	if fresh.Code != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("fresh request = %d/%q, calls=%d", fresh.Code, fresh.Body.String(), calls.Load())
	}
}

type requestDeadlineGateway04381 struct {
	calls      atomic.Int32
	canceled   chan struct{}
	cancelOnce sync.Once
	started    chan bool
}

func (gateway *requestDeadlineGateway04381) Execute(ctx context.Context, _ proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	gateway.calls.Add(1)
	_, hasDeadline := ctx.Deadline()
	gateway.started <- hasDeadline
	<-ctx.Done()
	gateway.cancelOnce.Do(func() { close(gateway.canceled) })
	return nil, ctx.Err()
}

func TestProdex04381StandardRequestDeadlineCoversUpstreamAndReturns504(t *testing.T) {
	gateway := &requestDeadlineGateway04381{canceled: make(chan struct{}), started: make(chan bool, 1)}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:                 gateway,
		PrecommitWatchdogBudget: 40 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "deadline-profile", Home: "/synthetic", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	responseDone := make(chan *http.Response, 1)
	errorDone := make(chan error, 1)
	go func() {
		response, err := http.Get(server.URL + "/backend-api/status")
		responseDone <- response
		errorDone <- err
	}()
	select {
	case hasDeadline := <-gateway.started:
		if !hasDeadline {
			t.Fatal("upstream context lost request deadline")
		}
	case <-time.After(time.Second):
		t.Fatal("upstream request did not start")
	}
	response := <-responseDone
	err = <-errorDone
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusGatewayTimeout ||
		!strings.Contains(string(body), `"code":"request_timeout"`) ||
		!strings.Contains(string(body), requestRecoveryTimeoutMessage) {
		t.Fatalf("standard timeout = %d/%q", response.StatusCode, body)
	}
	select {
	case <-gateway.canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request was not canceled at the request deadline")
	}
	if got := gateway.calls.Load(); got != 1 {
		t.Fatalf("upstream attempts = %d, want one", got)
	}
}
