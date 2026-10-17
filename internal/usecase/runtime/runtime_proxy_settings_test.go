package runtime

import (
	"strings"
	"testing"
	"time"
)

func TestRuntimeProxySettingsReadCanonicalAndLegacyEnvironment(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{
			"GODEX_RUNTIME_PROXY_ACTIVE_REQUEST_LIMIT":            "1",
			"PRODEX_RUNTIME_PROXY_ACTIVE_REQUEST_LIMIT":           "9",
			"PRODEX_RUNTIME_PROXY_ADMISSION_WAIT_BUDGET_MS":       "17",
			"GODEX_RUNTIME_PROXY_LONG_LIVED_QUEUE_WAIT_BUDGET_MS": "23",
		}
		value, ok := values[key]
		return value, ok
	}

	settings, err := runtimeProxySettingsWithLookup(lookup)
	if err != nil {
		t.Fatal(err)
	}
	if settings.activeRequestLimit != 1 || settings.admissionWaitBudget != 17*time.Millisecond ||
		settings.longLivedQueueWaitBudget != 23*time.Millisecond ||
		settings.pressureAdmissionWaitBudget != 200*time.Millisecond {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestRuntimeProxySettingsRejectInvalidValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "zero limit", key: "GODEX_RUNTIME_PROXY_ACTIVE_REQUEST_LIMIT", value: "0", want: "greater than zero"},
		{name: "bad duration", key: "GODEX_RUNTIME_PROXY_ADMISSION_WAIT_BUDGET_MS", value: "slow", want: "unsigned integer"},
		{name: "zero duration", key: "GODEX_RUNTIME_PROXY_LONG_LIVED_QUEUE_WAIT_BUDGET_MS", value: "0", want: "greater than zero"},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings, err := runtimeProxySettingsWithLookup(func(key string) (string, bool) {
				if key == test.key {
					return test.value, true
				}
				return "", false
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("settings=%#v err=%v, want %q", settings, err, test.want)
			}
		})
	}
}

// Prodex 0.438.2's published `PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT`
// knob is observable: idle completed sockets must release weight=2, and a
// later response reacquires exactly one weighted permit. Keep the Godex
// canonical spelling and source-compatible Prodex alias, both validated.
func TestProdex04382ProfileInflightHardLimitEnvironmentMatchesTaggedPolicy(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		want   int
		bad    bool
	}{
		{"default retains router policy", nil, 0, false},
		{"Prodex-compatible alias", map[string]string{"PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT": "2"}, 2, false},
		{"Godex canonical precedence", map[string]string{
			"GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT":  "3",
			"PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT": "9",
		}, 3, false},
		{"zero denied", map[string]string{"GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT": "0"}, 0, true},
		{"negative denied", map[string]string{"GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT": "-1"}, 0, true},
		{"text denied", map[string]string{"PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT": "invalid"}, 0, true},
		{"integer overflow denied", map[string]string{"PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT": "18446744073709551615"}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings, err := runtimeProxySettingsWithLookup(func(key string) (string, bool) { v, ok := tc.values[key]; return v, ok })
			if tc.bad {
				if err == nil {
					t.Fatalf("invalid hard limit accepted: %+v", settings)
				}
				return
			}
			if err != nil || settings.profileInflightHardLimit != tc.want {
				t.Fatalf("profile inflight hard limit=%d, want=%d err=%v", settings.profileInflightHardLimit, tc.want, err)
			}
		})
	}
}
