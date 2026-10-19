package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04383OnlyFirstRequestIncludesConnectionQueueAge(t *testing.T) {
	parent := proxyConnContext(t.Context(), acceptedAtConnection{acceptedAt: time.Now().Add(-time.Hour)})
	first, cancelFirst := withRequestDeadline(parent, time.Second)
	defer cancelFirst()
	if !requestDeadlineTimedOut(first, first.Err()) {
		t.Fatal("first request lost time spent waiting after connection acceptance")
	}
	before := time.Now()
	second, cancelSecond := withRequestDeadline(parent, time.Second)
	defer cancelSecond()
	scope, ok := requestDeadlineScopeFor(second)
	if !ok || second.Err() != nil || scope.deadline.Before(before.Add(time.Second)) {
		t.Fatal("next request inherited the preceding request's expired budget")
	}
}

func TestProdex04383HTTPKeepAliveRenewsEachRequestBudget(t *testing.T) {
	var calls atomic.Int32
	gateway := gatewayFunc(func(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error) {
		calls.Add(1)
		return &proxymodel.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp-keepalive"}`))}, nil
	})
	budget := 150 * time.Millisecond
	router, err := routingusecase.NewRouter(routingusecase.Config{
		Gateway: gateway, PrecommitWatchdogBudget: budget,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "synthetic-a", Home: "/synthetic", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(Config{Router: router})
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	defer proxy.Close(context.Background())
	transport := &http.Transport{MaxIdleConnsPerHost: 1}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	connections := make(chan httptrace.GotConnInfo, 2)
	for index := range 2 {
		ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { connections <- info }})
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, proxy.Endpoint()+"/backend-api/godex/responses", strings.NewReader(`{"input":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		info := <-connections
		if readErr != nil || response.StatusCode != 200 || index == 1 && !info.Reused {
			t.Fatalf("request %d status=%d reused=%t err=%v", index+1, response.StatusCode, info.Reused, readErr)
		}
		if index == 0 {
			<-time.NewTimer(2 * budget).C
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream attempts=%d, want 2", calls.Load())
	}
}
