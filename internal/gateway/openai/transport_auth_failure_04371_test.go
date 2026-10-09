package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type failedAuthReader struct{ err error }

func (reader failedAuthReader) ReadAuth(context.Context, string) (proxymodel.Auth, error) {
	return proxymodel.Auth{}, reader.err
}

func TestReadAuthFailureIsBoundedAndDoesNotReachUpstream(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		upstreamCalls++
	}))
	defer upstream.Close()

	transport, err := NewTransport(upstream.URL, nil, failedAuthReader{err: errors.New("auth file is unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	_, err = transport.Execute(t.Context(), proxymodel.Request{
		Method: http.MethodPost, Path: "/backend-api/prodex/responses", Header: make(http.Header),
	}, proxymodel.Account{ID: "profile-a", Home: "/profile-a", Enabled: true})
	var routeErr *proxymodel.Error
	if !errors.As(err, &routeErr) || routeErr.StatusCode != http.StatusBadGateway ||
		routeErr.Message != "proxied request could not be prepared" {
		t.Fatalf("auth read error = %v, want bounded HTTP 502 preparation error", err)
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstream calls = %d, want none", upstreamCalls)
	}
}
