package main

import (
	"testing"
	"time"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04382RuntimeOpenAITransportRetainsWebSocketStreamIdleSetting(t *testing.T) {
	config := proxyconfig.Config{UpstreamURL: "http://127.0.0.1:1", WebSocketCommittedIdleTimeout: 250 * time.Millisecond}
	source, err := newRuntimeGateway(config, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	timeout, ok := source.(interface{ CommittedWebSocketIdleTimeout04382() time.Duration })
	if !ok {
		t.Fatalf("runtime gateway exposes no typed configured WebSocket idle timeout: %T", source)
	}
	if got := timeout.CommittedWebSocketIdleTimeout04382(); got != 250*time.Millisecond {
		t.Fatalf("runtime committed idle timeout=%s want 250ms", got)
	}
}
