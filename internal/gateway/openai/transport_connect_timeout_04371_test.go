package openai

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestProdex04371UpstreamDialUsesBoundedConnectDeadline(t *testing.T) {
	var deadline time.Time
	base := func(ctx context.Context, _, _ string) (net.Conn, error) {
		deadline, _ = ctx.Deadline()
		return nil, errors.New("synthetic dial failure")
	}
	client := cloneHTTPClient(&http.Client{Transport: &http.Transport{DialContext: base}})
	transport := client.Transport.(*http.Transport)
	_, err := transport.DialContext(context.Background(), "tcp", "synthetic.invalid:443")
	if err == nil {
		t.Fatal("synthetic dial unexpectedly succeeded")
	}
	if remaining := time.Until(deadline); remaining < upstreamConnectTimeout-time.Second || remaining > upstreamConnectTimeout {
		t.Fatalf("upstream dial deadline = %s from now, want at most %s", remaining, upstreamConnectTimeout)
	}
	if transport.TLSHandshakeTimeout != upstreamConnectTimeout {
		t.Fatalf("TLS handshake timeout = %s, want %s", transport.TLSHandshakeTimeout, upstreamConnectTimeout)
	}
}
