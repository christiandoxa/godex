package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestResponseHeaderFilteringHandlesNonCanonicalConnectionKeys(t *testing.T) {
	destination := make(http.Header)
	source := http.Header{
		"connection":  {"x-local-hop"},
		"x-local-hop": {"strip-me"},
		"x-codex":     {"keep-me"},
	}
	copyResponseHeaders(destination, source)
	if destination.Get("x-local-hop") != "" || destination.Get("x-codex") != "keep-me" {
		t.Fatalf("filtered response headers = %#v", destination)
	}
}

func TestResponseTrailerDeclarationHandlesNonCanonicalTrailerKey(t *testing.T) {
	destination := make(http.Header)
	declareResponseTrailers(destination, http.Header{"trailer": {"x-upstream"}}, nil)
	if destination.Get("Trailer") != "X-Upstream" {
		t.Fatalf("declared trailers = %#v", destination)
	}
}

func TestProxyAddsValidDateWhenUpstreamHasNoDate(t *testing.T) {
	proxy := &Proxy{maxInspect: 4096}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxy.forwardResponse(context.Background(), w, &proxymodel.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil, "", &requestLifecycle{})
	}))
	defer server.Close()
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got := response.Header.Get("Date")
	date, err := http.ParseTime(got)
	if err != nil || time.Since(date) > 1*time.Minute || time.Until(date) > 1*time.Minute {
		t.Fatalf("missing or stale server Date header: %q, err=%v", got, err)
	}
}
