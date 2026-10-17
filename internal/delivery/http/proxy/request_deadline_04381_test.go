package proxy

import (
	"context"
	"errors"
	"io"
	"net"
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

func TestProdex04381RequestDeadlineIncludesAcceptedConnectionTime(t *testing.T) {
	acceptedAt := time.Now().Add(-500 * time.Millisecond)
	parent := context.WithValue(context.Background(), requestAcceptedAtContextKey{}, acceptedAt)
	ctx, cancel := withRequestDeadline(parent, 100*time.Millisecond)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || deadline.Sub(acceptedAt.Add(100*time.Millisecond)) > time.Millisecond ||
		deadline.Sub(acceptedAt.Add(100*time.Millisecond)) < -time.Millisecond {
		t.Fatalf("deadline=%v, want accepted-at deadline near %v", deadline, acceptedAt.Add(100*time.Millisecond))
	}
	if !requestDeadlineTimedOut(ctx) {
		t.Fatal("deadline that elapsed in the listener queue was not expired")
	}
}

func TestProdex04381ProxyConnContextCarriesAcceptedTime(t *testing.T) {
	acceptedAt := time.Now().Add(-time.Second)
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	ctx := proxyConnContext(context.Background(), acceptedAtConnection{Conn: client, acceptedAt: acceptedAt})
	got, ok := requestAcceptedAt(ctx)
	if !ok || !got.Equal(acceptedAt) {
		t.Fatalf("accepted time=%v/%t, want %v/true", got, ok, acceptedAt)
	}
}

func TestProdex04381AcceptedTimeReachesHTTPRequestContext(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan time.Time, 1)
	server := &http.Server{
		ConnContext: proxyConnContext,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			at, _ := requestAcceptedAt(request.Context())
			accepted <- at
			writer.Header().Set("Connection", "close")
			_, _ = io.WriteString(writer, "ok")
		}),
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(acceptedAtListener{Listener: listener}) }()
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	_, _ = connection.Write([]byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"))
	_, _ = io.ReadAll(connection)
	_ = connection.Close()
	select {
	case at := <-accepted:
		if at.IsZero() {
			t.Fatal("HTTP request lost accepted connection time")
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not reach handler")
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownContext)
	select {
	case serveErr := <-serveDone:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			t.Fatal(serveErr)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP server did not stop")
	}
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

func TestProdex04381ExpiredAdmissionCannotTakeReleasedPermit(t *testing.T) {
	handler, ok := newActiveRequestHandlerWithLimits(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), oneLaneAdmissionLimits04381(),
	).(*activeRequestHandler)
	if !ok {
		t.Fatal("admission handler type changed")
	}
	if !handler.acquire(context.Background(), admissionLaneResponses) {
		t.Fatal("failed to hold admission permit")
	}
	waiting := make(chan struct{})
	handler.wait = func(ctx context.Context, changed <-chan struct{}) bool {
		close(waiting)
		select {
		case <-changed:
			return true
		case <-ctx.Done():
			return false
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	acquired := make(chan bool, 1)
	go func() { acquired <- handler.acquire(ctx, admissionLaneResponses) }()
	<-waiting
	cancel()
	handler.release(admissionLaneResponses)
	if <-acquired {
		t.Fatal("expired waiter acquired a permit released after cancellation")
	}
	if handler.active != 0 || handler.laneActive[admissionLaneResponses] != 0 {
		t.Fatalf("permit leaked after expired admission: active=%d lane=%d", handler.active, handler.laneActive[admissionLaneResponses])
	}
}

type requestDeadlineGateway04381 struct {
	calls      atomic.Int32
	canceled   chan struct{}
	cancelOnce sync.Once
	started    chan bool
}

type blockingBody04381 struct {
	closed chan struct{}
	once   sync.Once
}

func (body *blockingBody04381) Read([]byte) (int, error) {
	<-body.closed
	return 0, io.ErrClosedPipe
}

func (body *blockingBody04381) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

func TestProdex04381BodyCaptureUsesRequestDeadline(t *testing.T) {
	gateway := &requestDeadlineGateway04381{
		canceled: make(chan struct{}),
		started:  make(chan bool, 1),
	}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:                 gateway,
		PrecommitWatchdogBudget: 35 * time.Millisecond,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "body-deadline-profile", Home: "/synthetic", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/backend-api/status", nil)
	request.Body = &blockingBody04381{closed: make(chan struct{})}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout ||
		!strings.Contains(response.Body.String(), `"code":"request_timeout"`) {
		t.Fatalf("body capture timeout = %d/%q", response.Code, response.Body.String())
	}
	if gateway.calls.Load() != 0 {
		t.Fatalf("expired body capture dispatched upstream %d times", gateway.calls.Load())
	}
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
