package proxy

import (
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

type copilotLateEvent04359Reader struct{ stage int }

func (r *copilotLateEvent04359Reader) Read(buffer []byte) (int, error) {
	const first = `data: {"type":"response.output_text.delta","delta":"answer"}

`
	const second = `data: {"type":"response.completed","message":{"id":"copilot-late-id"}}

`
	switch r.stage {
	case 0:
		r.stage = 1
		return copy(buffer, first), nil
	case 1:
		r.stage = 2
		return copy(buffer, second), nil
	default:
		return 0, io.EOF
	}
}
func (*copilotLateEvent04359Reader) Close() error { return nil }

type copilotLateEvent04359Gateway struct {
	mu     sync.Mutex
	owners []string
}

func (g *copilotLateEvent04359Gateway) Execute(_ context.Context, req proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	g.mu.Lock()
	g.owners = append(g.owners, account.ID)
	g.mu.Unlock()
	if strings.Contains(string(req.Body), "previous_response_id") {
		return &proxymodel.Response{StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": []string{"application/json"}},
			Body:   io.NopCloser(strings.NewReader(`{"object":"response","id":"continued"}`))}, nil
	}
	return &proxymodel.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   &copilotLateEvent04359Reader{}}, nil
}

// Exact Prodex 0.435.9 Copilot provider bindings must survive after the
// initial SSE commitment. Only the actual committed stream reader sees this
// message.id; a payload-preservation-only unit test would miss the defect.
func TestProdex04359CopilotLateSSEEventBindsContinuationThroughPublicProxy(t *testing.T) {
	gateway := &copilotLateEvent04359Gateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway:          gateway,
		PreferredAccount: "copilot-a",
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{
				{ID: "copilot-a", Enabled: true, Home: "/a", Provider: proxymodel.Provider{Kind: "copilot"}},
				{ID: "copilot-b", Enabled: true, Home: "/b", Provider: proxymodel.Provider{Kind: "copilot"}},
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	proxy.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/backend-api/godex/responses", strings.NewReader(`{"input":"hello"}`)))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "copilot-late-id") {
		t.Fatalf("first response %d/%q", first.Code, first.Body.String())
	}
	// An unknown previous response must fail closed. A continuation after this
	// late Copilot SSE ID succeeds only if the proxy recorded its affinity.
	router.SetPreferredAccount("copilot-b")
	second := httptest.NewRecorder()
	proxy.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/backend-api/godex/responses",
		strings.NewReader(`{"previous_response_id":"copilot-late-id","input":"continue"}`)))
	if second.Code != http.StatusOK {
		t.Fatalf("late continuation status=%d body=%s", second.Code, second.Body.String())
	}
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.owners) != 2 || gateway.owners[0] != "copilot-a" || gateway.owners[1] != "copilot-a" {
		t.Fatalf("late response ID did not pin its originating account: owners=%v", gateway.owners)
	}
}
