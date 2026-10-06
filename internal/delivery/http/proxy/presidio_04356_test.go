package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type presidioRecordingRedactor struct {
	mu   sync.Mutex
	seen [][]byte
	err  error
}

func (redactor *presidioRecordingRedactor) Redact(_ context.Context, body []byte) ([]byte, error) {
	redactor.mu.Lock()
	redactor.seen = append(redactor.seen, append([]byte(nil), body...))
	redactor.mu.Unlock()
	if redactor.err != nil {
		return nil, redactor.err
	}
	return append([]byte(nil), body...), nil
}

func (redactor *presidioRecordingRedactor) snapshot() [][]byte {
	redactor.mu.Lock()
	defer redactor.mu.Unlock()
	result := make([][]byte, len(redactor.seen))
	for index := range redactor.seen {
		result[index] = append([]byte(nil), redactor.seen[index]...)
	}
	return result
}

func TestProdex04356PresidioHTTPRunsBeforeSmartContextAndRouting(t *testing.T) {
	gateway := &smartContextCaptureGateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "A", Home: t.TempDir(), Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	redactor := &presidioRecordingRedactor{}
	proxy, err := NewProxy(Config{
		Router: router, ListenAddr: "127.0.0.1:0",
		SmartContextEnabled: true, Redactor: redactor,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	long := strings.Repeat("presidio-before-smart duplicate ", 500)
	original := smartContextFixture("gpt-5.4", []any{
		messageInput("user", long),
		messageInput("user", long),
	})
	response := doProxyJSON(t, server.URL+"/backend-api/godex/responses", string(original), nil)
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	seen := redactor.snapshot()
	if len(seen) != 1 || !bytes.Equal(seen[0], original) {
		t.Fatalf("redactor did not receive original captured body: %#v", seen)
	}
	routed := gateway.snapshot()
	if len(routed) != 1 || bytes.Equal(routed[0], original) ||
		!bytes.Contains(routed[0], []byte("[godex-context-ref ")) {
		t.Fatalf("router body = %#v", routed)
	}
}

func TestProdex04356PresidioHTTPFailureReturns502BeforeRouting(t *testing.T) {
	gateway := &smartContextCaptureGateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "A", Home: t.TempDir(), Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{
		Router: router, ListenAddr: "127.0.0.1:0",
		Redactor: &presidioRecordingRedactor{err: errors.New("synthetic Presidio failure")},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	response := doProxyJSON(t, server.URL+"/backend-api/godex/responses", `{"input":"sensitive"}`, nil)
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadGateway ||
		!strings.Contains(string(body), "gateway PII redaction failed") {
		t.Fatalf("failure response = status:%d body:%q", response.StatusCode, body)
	}
	if routed := gateway.snapshot(); len(routed) != 0 {
		t.Fatalf("failed Presidio request reached router: %#v", routed)
	}
}

func TestProdex04356PresidioWebSocketRunsBeforeSmartContextAndRouting(t *testing.T) {
	gateway := &responsesPublicGateway{closed: make(chan uint64, 1)}
	proxy := newResponsesPublicProxy(t, gateway)
	redactor := &presidioRecordingRedactor{}
	proxy.redactor = redactor
	proxy.smartContextEnabled = true
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/codex/responses")
	defer connection.Close()
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d", status)
	}

	long := strings.Repeat("presidio websocket duplicate ", 500)
	payload := smartContextFixture("gpt-5.4", []any{
		messageInput("user", long),
		messageInput("user", long),
	})
	if _, err := connection.Write(protocolClientFrame(1, true, payload, true)); err != nil {
		t.Fatal(err)
	}
	_ = readResponsesPublicTextFrame(t, reader)

	seen := redactor.snapshot()
	if len(seen) != 1 || !bytes.Equal(seen[0], payload) {
		t.Fatalf("websocket redactor input = %#v", seen)
	}
	gateway.mu.Lock()
	bodies := append([]string(nil), gateway.bodies...)
	gateway.mu.Unlock()
	if len(bodies) != 1 || bodies[0] == string(payload) ||
		!strings.Contains(bodies[0], "[godex-context-ref ") {
		t.Fatalf("websocket routed bodies = %#v", bodies)
	}
}

func TestProdex04356PresidioWebSocketFailureDoesNotRouteFrame(t *testing.T) {
	gateway := &responsesPublicGateway{closed: make(chan uint64, 1)}
	proxy := newResponsesPublicProxy(t, gateway)
	proxy.redactor = &presidioRecordingRedactor{err: errors.New("synthetic Presidio failure")}
	server := httptest.NewServer(proxy.server.Handler)
	defer server.Close()

	connection, reader := dialResponsesPublicWebSocket(t, server.URL, "/backend-api/codex/responses")
	defer connection.Close()
	status, _ := readResponsesPublicHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d", status)
	}
	payload := []byte(`{"type":"response.create","input":"sensitive"}`)
	if _, err := connection.Write(protocolClientFrame(1, true, payload, true)); err != nil {
		t.Fatal(err)
	}
	message := readResponsesPublicTextFrame(t, reader)
	if !strings.Contains(message, `"code":"presidio_redaction_failed"`) ||
		!strings.Contains(message, `"status":502`) {
		t.Fatalf("redaction websocket error = %q", message)
	}
	gateway.mu.Lock()
	calls := len(gateway.bodies)
	gateway.mu.Unlock()
	if calls != 0 {
		t.Fatalf("failed Presidio websocket frame reached router: %d calls", calls)
	}
}
