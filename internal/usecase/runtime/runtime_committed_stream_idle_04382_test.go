package runtime

import (
	"context"
	"testing"
	"time"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04382RuntimeLaunchCarriesConfiguredWebSocketCommittedIdle(t *testing.T) {
	t.Setenv("GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS", "250")
	t.Setenv("PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS", "8000")
	config, err := runtimeProxyConfig(context.Background(), "http://127.0.0.1", "synthetic-owner", proxyconfig.Provider{}, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if config.WebSocketCommittedIdleTimeout != 250*time.Millisecond || config.PrecommitWatchdogBudget != 30*time.Second {
		t.Fatalf("wrong independently-owned stream timeouts: committed=%s precommit=%s", config.WebSocketCommittedIdleTimeout, config.PrecommitWatchdogBudget)
	}
}
