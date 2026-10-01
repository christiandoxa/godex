package runtime

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
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
	if selector.Account != "work" || selector.Profile != "" {
		t.Fatalf("selection = %+v", selector)
	}
	want := []string{
		"-c", `web_search="indexed"`,
		"-c", "features.rollout_budget.enabled=true",
		"-c", "features.rollout_budget.limit_tokens=100000",
		"-c", "features.rollout_budget.reminder_at_remaining_tokens=[75000,50000,25000]",
		"-c", "features.rollout_budget.sampling_token_weight=1.5",
		"-c", "features.rollout_budget.prefill_token_weight=0.25",
		"-c", "features.current_time_reminder.enabled=true",
		"-c", "features.current_time_reminder.reminder_interval_seconds=2",
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
		{"--rollout-budget-tokens", "9223372036854775808"},
		{"--rollout-budget-tokens", "100", "--rollout-budget-reminders", "18446744073709551615"},
		{"--current-time-reminder-interval", "9223372036854775808"},
		{"--rollout-budget-tokens", "100", "--rollout-budget-sampling-weight", "NaN"},
		{"--rollout-budget-tokens", "100", "--rollout-budget-prefill-weight", "Inf"},
		{"--rollout-budget-tokens", "100", "--rollout-budget-prefill-weight", "-1"},
	} {
		if _, _, err := parseRunArguments(arguments); err == nil {
			t.Fatalf("arguments %#v unexpectedly succeeded", arguments)
		}
	}
}

func TestLargeRolloutBudgetRemindersDoNotOverflow(t *testing.T) {
	if got, want := normalizeReminders(1<<60, nil), []uint64{3 << 58, 1 << 59, 1 << 58}; !reflect.DeepEqual(got, want) {
		t.Fatalf("large budget reminders = %v, want %v", got, want)
	}
}

// Closed stdio validates the wrapper's actual output without accepting any work.
func TestInstalledCodexRuntimeFeaturesSmoke(t *testing.T) {
	binary := os.Getenv("GODEX_TEST_CODEX_BIN")
	if binary == "" {
		t.Skip("set GODEX_TEST_CODEX_BIN for a local parser smoke")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, arguments, err := parseRunArguments([]string{
		"--web-search", "indexed", "--rollout-budget-tokens", "100000",
		"--rollout-budget-reminders", "75000,50000,25000",
		"--rollout-budget-sampling-weight", "1.5", "--rollout-budget-prefill-weight", "0.25",
		"--current-time-reminder", "--current-time-reminder-interval", "2",
		"--current-time-clock-source", "system", "--respect-system-proxy",
		"exec-server", "--listen", "stdio",
	})
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	command := exec.CommandContext(ctx, binary, append([]string{"--strict-config"}, arguments...)...)
	command.Dir = home
	command.Env = append(os.Environ(), "CODEX_HOME="+home)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("runtime features failed native config validation: %v, %s", err, output)
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
