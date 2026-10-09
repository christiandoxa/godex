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

func TestProxyRecomputesBufferedJSONContentLength(t *testing.T) {
	proxy := &Proxy{maxInspect: 1024}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxy.forwardResponse(context.Background(), w, &proxymodel.Response{
			StatusCode: http.StatusOK,
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
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` || response.ContentLength != int64(len(body)) {
		t.Fatalf("incorrect buffered response framing: length=%d body=%q", response.ContentLength, body)
	}
}

func TestProxyDoesNotInventLengthForPartiallyBufferedBody(t *testing.T) {
	proxy := &Proxy{maxInspect: 3}
	payload := strings.Repeat("x", 4096)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxy.forwardResponse(context.Background(), w, &proxymodel.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "Content-Length": []string{"999"}},
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil, "", &requestLifecycle{})
	}))
	defer server.Close()
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != payload || response.ContentLength != -1 {
		t.Fatalf("partially inspected response got invalid framing: length=%d body=%q", response.ContentLength, body)
	}
}

// Streams use their translated event-stream headers and do not add a
// synthetic Date that is absent in the tagged Prodex proxy response.
func TestProdex04361SSEHeadersDoNotSynthesizeDate(t *testing.T) {
	proxy := &Proxy{maxInspect: 4096}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		proxy.forwardResponse(context.Background(), writer, &proxymodel.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader("event: response.completed\r\ndata: {}\r\n\r\n")),
		}, nil, "", &requestLifecycle{})
	}))
	defer server.Close()
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	wire, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Date") != "" || response.Header.Get("Content-Type") != "text/event-stream; charset=utf-8" {
		t.Fatalf("SSE response headers drifted: %#v", response.Header)
	}
	if !strings.Contains(string(wire), "event: response.completed") {
		t.Fatalf("SSE event body truncated: %q", wire)
	}
}
