package runtime

import (
	"reflect"
	"testing"
)

func TestRunFeatureFlagsRenderCodexOverrides(t *testing.T) {
	selector, arguments, err := parseRunArguments([]string{
		"--account", "work",
		"--web-search", "indexed",
		"--rollout-budget-tokens", "100000",
		"--rollout-budget-reminders", "75000,50000,25000",
		"--rollout-budget-sampling-weight", "1.5",
		"--rollout-budget-prefill-weight", "0.25",
		"--current-time-reminder",
		"--current-time-reminder-interval", "2",
		"--current-time-clock-source", "system",
		"--respect-system-proxy",
		"exec", "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if selector != "work" {
		t.Fatalf("selector = %q", selector)
	}
	want := []string{
		"-c", `web_search="indexed"`,
		"-c", "features.rollout_budget.enabled=true",
		"-c", "features.rollout_budget.limit_tokens=100000",
		"-c", "features.rollout_budget.reminder_at_remaining_tokens=[75000,50000,25000]",
		"-c", "features.rollout_budget.sampling_token_weight=1.5",
		"-c", "features.rollout_budget.prefill_token_weight=0.25",
		"-c", "features.current_time_reminder.enabled=true",
		"-c", "features.current_time_reminder.reminder_interval_model_requests=2",
		"-c", `features.current_time_reminder.clock_source="system"`,
		"-c", "features.respect_system_proxy=true",
		"exec", "hello",
	}
	if !reflect.DeepEqual(arguments, want) {
		t.Fatalf("arguments = %#v\nwant %#v", arguments, want)
	}
}

func TestRolloutBudgetUsesProdexDefaultReminderThresholds(t *testing.T) {
	_, arguments, err := parseRunArguments([]string{"--rollout-budget-tokens=100000", "exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	want := "features.rollout_budget.reminder_at_remaining_tokens=[75000,50000,25000]"
	if !containsArgument(arguments, want) {
		t.Fatalf("arguments = %#v", arguments)
	}
}

func TestRuntimeFeatureParsingRejectsInvalidWrapperOptions(t *testing.T) {
	for _, arguments := range [][]string{
		{"--web-search", "unknown"},
		{"--rollout-budget-tokens", "not-a-number"},
		{"--rollout-budget-reminders", "10,broken"},
		{"--rollout-budget-reminders", "10"},
		{"--current-time-clock-source", "network"},
		{"--respect-system-proxy", "--no-respect-system-proxy"},
	} {
		if _, _, err := parseRunArguments(arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly succeeded", arguments)
		}
	}
}

func TestFeatureOverridesPrecedeExplicitCodexConfiguration(t *testing.T) {
	_, arguments, err := parseRunArguments([]string{"--web-search", "cached", "-c", `web_search="live"`, "exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-c", `web_search="cached"`, "-c", `web_search="live"`, "exec", "hello"}
	if !reflect.DeepEqual(arguments, want) {
		t.Fatalf("arguments = %#v, want %#v", arguments, want)
	}
}

func TestRespectSystemProxyDisableOverride(t *testing.T) {
	_, arguments, err := parseRunArguments([]string{"--no-respect-system-proxy", "exec", "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsArgument(arguments, "features.respect_system_proxy=false") {
		t.Fatalf("arguments = %#v", arguments)
	}
}

func containsArgument(arguments []string, want string) bool {
	for _, argument := range arguments {
		if argument == want {
			return true
		}
	}
	return false
}
