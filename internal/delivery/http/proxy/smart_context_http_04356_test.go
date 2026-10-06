package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type smartContextCaptureGateway struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (gateway *smartContextCaptureGateway) Execute(
	_ context.Context,
	request proxymodel.Request,
	_ proxymodel.Account,
) (*proxymodel.Response, error) {
	gateway.mu.Lock()
	gateway.bodies = append(gateway.bodies, append([]byte(nil), request.Body...))
	gateway.mu.Unlock()
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"resp-smart"}`)),
	}, nil
}

func (gateway *smartContextCaptureGateway) snapshot() [][]byte {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	result := make([][]byte, len(gateway.bodies))
	for index := range gateway.bodies {
		result[index] = append([]byte(nil), gateway.bodies[index]...)
	}
	return result
}

func TestProdex04356SmartContextHTTPProxyRewritesOnceBeforeRouting(t *testing.T) {
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
		Router: router, ListenAddr: "127.0.0.1:0", SmartContextEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	long := strings.Repeat("http integration duplicate ", 500)
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
	bodies := gateway.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("gateway body count = %d", len(bodies))
	}
	if !strings.Contains(string(bodies[0]), "[godex-context-ref ") ||
		!strings.Contains(string(bodies[0]), smartContextInlineReferenceProtocol) {
		t.Fatalf("gateway did not receive rewritten Smart Context body: %s", bodies[0])
	}
	if bytes.Equal(bodies[0], original) {
		t.Fatal("gateway received original body despite eligible rewrite")
	}
}

func TestProdex04356SmartContextHTTPProxyExactHeaderPreservesOriginalBytes(t *testing.T) {
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
		Router: router, ListenAddr: "127.0.0.1:0", SmartContextEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()

	long := strings.Repeat("exact integration duplicate ", 500)
	original := smartContextFixture("gpt-5.4", []any{
		messageInput("user", long),
		messageInput("user", long),
	})
	response := doProxyJSON(t, server.URL+"/backend-api/godex/responses", string(original), map[string]string{
		"X-Godex-Smart-Context": "exact",
	})
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	bodies := gateway.snapshot()
	if len(bodies) != 1 || !bytes.Equal(bodies[0], original) {
		t.Fatalf("exact body changed: %q", bodies)
	}
}
