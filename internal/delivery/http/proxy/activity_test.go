package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

type recordingActivity struct {
	events []runtimemodel.Event
}

func (activity *recordingActivity) Record(_ context.Context, event runtimemodel.Event) error {
	activity.events = append(activity.events, event)
	return nil
}

func TestProxyRecordsRedactedRuntimeActivity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"response-one"}`))
	}))
	defer upstream.Close()
	accounts := testRuntimeAccounts(t, "A", "synthetic-secret-token", "B", "token-b")
	recorder := &recordingActivity{}
	proxy, err := newProxyForTest(ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: upstream.URL, Activity: recorder,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	response := doProxyJSON(t, server.URL+"/responses", `{"input":"private prompt"}`, nil)
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if len(recorder.events) != 2 {
		t.Fatalf("events = %#v", recorder.events)
	}
	if recorder.events[0].Kind != "request_started" || recorder.events[1].Kind != "request_completed" {
		t.Fatalf("event kinds = %#v", recorder.events)
	}
	if recorder.events[1].StatusCode != http.StatusOK || recorder.events[1].AccountID != "A" {
		t.Fatalf("completion = %#v", recorder.events[1])
	}
	encoded, err := json.Marshal(recorder.events)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"synthetic-secret-token", "private prompt"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("runtime activity leaked %q", secret)
		}
	}
}

func TestProxyRecordsPrecommitFailure(t *testing.T) {
	recorder := &recordingActivity{}
	accounts := testRuntimeAccounts(t, "A", "token-a", "B", "token-b")
	proxy, err := newProxyForTest(ProxyConfig{
		ListenAddr: "127.0.0.1:0", UpstreamURL: "http://127.0.0.1:1", Activity: recorder,
		Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(proxy)
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/backend-api/codex/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Upgrade", "websocket")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if len(recorder.events) != 2 || recorder.events[1].Kind != "request_failed" || recorder.events[1].StatusCode != http.StatusBadRequest {
		t.Fatalf("failure events = %#v", recorder.events)
	}
}

type requestIDCaptureGateway struct {
	ids []uint64
}

func (gateway *requestIDCaptureGateway) Execute(_ context.Context, request proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	gateway.ids = append(gateway.ids, request.RequestID)
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"response-id"}`)),
	}, nil
}

func TestProxyForwardsMonotonicRequestIDToGateway(t *testing.T) {
	gateway := &requestIDCaptureGateway{}
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
		response := doProxyJSON(t, server.URL+"/responses", `{}`, nil)
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
