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
