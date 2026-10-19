package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProdex04383TransportCapacityBlockerKeepsAnAuthenticatedStreamOpen(t *testing.T) {
	b := newTransportBackend04383("busy-history")
	defer b.server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.server.URL+"/responses", strings.NewReader(`{"mode":"capacity-blocker"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer token-b")
	request.Header.Set("ChatGPT-Account-Id", "account-b")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	select {
	case <-b.busy:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" || !b.occupied || b.invalid || len(b.attempts) != 0 || len(b.handshakes) != 0 {
		t.Fatal("capacity blocker did not retain a separate authenticated HTTP stream")
	}
}
