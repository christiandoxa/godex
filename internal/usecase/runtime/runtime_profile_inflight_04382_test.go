package runtime

import (
	"context"
	"testing"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

// The production launch path must carry the tagged 0.438.2 inflight setting
// to the actual Router config. A parser-only fixture is insufficient: under
// one-ready-profile pressure it would still silently use the default 8.
func TestProdex04382RuntimeProxyLaunchPropagatesProfileInflightHardLimit(t *testing.T) {
	t.Setenv("GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT", "2")
	t.Setenv("PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT", "9")
	got, err := runtimeProxyConfig(context.Background(), "http://127.0.0.1", "synthetic-profile",
		proxyconfig.Provider{}, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProfileInflightHardLimit != 2 {
		t.Fatalf("actual runtime launch hard limit=%d, want 2", got.ProfileInflightHardLimit)
	}
}
