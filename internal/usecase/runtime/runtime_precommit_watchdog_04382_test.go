package runtime

import (
	"context"
	"testing"
	"time"

	proxyconfig "github.com/christiandoxa/godex/internal/model/proxy"
)

// Prodex tagged 0.438.2 `recovery_watchdog_budget_ms` chooses
// max(configured stream idle ms, 30_000). This is request-local; it cannot be
// reset by hidden WebSocket keepalive/metadata or a profile reselection.
func TestProdex04382StreamIdleTuningBoundsOriginalPrecommitWatchdog(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		want   time.Duration
		bad    bool
	}{
		{name: "no override retains five-minute default", want: 0},
		{name: "legacy Prodex short timeout has thirty second floor", values: map[string]string{"PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "150"}, want: 30 * time.Second},
		{name: "exact epoch", values: map[string]string{"PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "30000"}, want: 30 * time.Second},
		{name: "above epoch", values: map[string]string{"GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "40000"}, want: 40 * time.Second},
		{name: "Godex precedence", values: map[string]string{"GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "12000", "PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "900000"}, want: 30 * time.Second},
		{name: "long watchdog", values: map[string]string{"PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "360000"}, want: 6 * time.Minute},
		{name: "reject zero", values: map[string]string{"PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "0"}, bad: true},
		{name: "reject negative", values: map[string]string{"GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "-17"}, bad: true},
		{name: "reject overflow", values: map[string]string{"PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS": "9223372036854775807"}, bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings, err := runtimeProxySettingsWithLookup(func(key string) (string, bool) { value, ok := tc.values[key]; return value, ok })
			if tc.bad {
				if err == nil {
					t.Fatalf("accepted invalid stream idle: %#v", settings)
				}
				return
			}
			if err != nil || settings.precommitWatchdogBudget != tc.want {
				t.Fatalf("precommit budget=%s, want=%s err=%v", settings.precommitWatchdogBudget, tc.want, err)
			}
		})
	}
}

func TestProdex04382RuntimeLaunchForwardsStreamIdleWatchdog(t *testing.T) {
	t.Setenv("GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS", "50")
	t.Setenv("PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS", "900000")
	config, err := runtimeProxyConfig(context.Background(), "http://127.0.0.1", "synthetic-owner", proxyconfig.Provider{}, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if config.PrecommitWatchdogBudget != 30*time.Second {
		t.Fatalf("runtime precommit watchdog=%s, want 30s", config.PrecommitWatchdogBudget)
	}
}
