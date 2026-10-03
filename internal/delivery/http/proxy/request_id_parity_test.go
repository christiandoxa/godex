package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type requestSequenceGateway struct{ ids []uint64 }

func (gateway *requestSequenceGateway) Execute(_ context.Context, request proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	gateway.ids = append(gateway.ids, request.RequestID)
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"response-id"}`)),
	}, nil
}

func TestProxyRequestSequenceReachesProvider(t *testing.T) {
	gateway := &requestSequenceGateway{}
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "A", Home: t.TempDir(), Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	for range 2 {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/responses", strings.NewReader(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", response.StatusCode)
		}
	}
	if len(gateway.ids) != 2 || gateway.ids[0] != 1 || gateway.ids[1] != 2 {
		t.Fatalf("gateway request IDs = %#v", gateway.ids)
	}
}
