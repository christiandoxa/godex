package runtime

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	rolloutBudgetTokensFlag         = "--rollout-budget-tokens"
	rolloutBudgetRemindersFlag      = "--rollout-budget-reminders"
	rolloutBudgetSamplingWeightFlag = "--rollout-budget-sampling-weight"
	rolloutBudgetPrefillWeightFlag  = "--rollout-budget-prefill-weight"
	currentTimeReminderIntervalFlag = "--current-time-reminder-interval"
)

type runtimeFeatures struct {
	webSearch          string
	nativeOptions      []string
	rolloutLimit       *uint64
	rolloutReminders   []uint64
	samplingWeight     *float64
	prefillWeight      *float64
	currentTime        bool
	currentInterval    *uint64
	currentClock       string
	respectSystemProxy *bool
}

func (features *runtimeFeatures) consume(arguments []string, index int) (next int, handled bool, err error) {
	switch arguments[index] {
	case "--no-presidio", "--no-sub-agent", "--no-auto-rotate", "--full-access":
		return index + 1, true, nil
	case "--presidio", "--sub-agent", "--auto-rotate", "--skip-quota-check", "--no-proxy":
		features.nativeOptions = append(features.nativeOptions, arguments[index])
		return index + 1, true, nil
	}
	for _, name := range []string{
		"--tool", "--require-tool", "--sub-agent-provider", "--sub-agent-model",
		"--sub-agent-model-reasoning-effort", "--sub-agent-url", "--sub-agent-max-concurrency",
	} {
		_, consumed, ok, err := namedOptionValue(arguments, index, name)
		if !ok {
			continue
		}
		if err != nil {
			return index, true, err
		}
		features.nativeOptions = append(features.nativeOptions, arguments[index:index+consumed]...)
		return index + consumed, true, nil
	}
	switch featureName(arguments[index]) {
	case "--web-search":
		return features.consumeWebSearch(arguments, index)
	case rolloutBudgetTokensFlag:
		return features.consumeRolloutLimit(arguments, index)
	case rolloutBudgetRemindersFlag:
		return features.consumeRolloutReminders(arguments, index)
	case rolloutBudgetSamplingWeightFlag:
		return features.consumeSamplingWeight(arguments, index)
	case rolloutBudgetPrefillWeightFlag:
		return features.consumePrefillWeight(arguments, index)
	case "--current-time-reminder":
		features.currentTime = true
		return index + 1, true, nil
	case currentTimeReminderIntervalFlag:
		return features.consumeCurrentInterval(arguments, index)
	case "--current-time-clock-source":
		return features.consumeCurrentClock(arguments, index)
	case "--respect-system-proxy", "--no-respect-system-proxy":
		return features.consumeSystemProxy(arguments, index)
	default:
		return index, false, nil
	}
}

func featureName(argument string) string {
	name, _, _ := strings.Cut(argument, "=")
	return name
}

func (features *runtimeFeatures) consumeWebSearch(arguments []string, index int) (int, bool, error) {
	value, consumed, _, err := featureValue(arguments, index, "--web-search")
	if err != nil {
		return index, true, err
	}
	switch value {
	case "disabled", "cached", "indexed", "live":
		features.webSearch = value
		return index + consumed, true, nil
	default:
		return index, true, fmt.Errorf("invalid --web-search value %q", value)
	}
}

func (features *runtimeFeatures) consumeRolloutLimit(arguments []string, index int) (int, bool, error) {
	value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetTokensFlag)
	parsed, err := parseUintFeature(rolloutBudgetTokensFlag, value, prior)
	if err != nil {
		return index, true, err
	}
	features.rolloutLimit = &parsed
	return index + consumed, true, nil
}

func (features *runtimeFeatures) consumeRolloutReminders(arguments []string, index int) (int, bool, error) {
	value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetRemindersFlag)
	parsed, err := parseUintListFeature(rolloutBudgetRemindersFlag, value, prior)
	if err != nil {
		return index, true, err
	}
	features.rolloutReminders = parsed
	return index + consumed, true, nil
}

func (features *runtimeFeatures) consumeSamplingWeight(arguments []string, index int) (int, bool, error) {
	value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetSamplingWeightFlag)
	parsed, err := parseFloatFeature(rolloutBudgetSamplingWeightFlag, value, prior)
	if err != nil {
		return index, true, err
	}
	features.samplingWeight = &parsed
	return index + consumed, true, nil
}

func (features *runtimeFeatures) consumePrefillWeight(arguments []string, index int) (int, bool, error) {
	value, consumed, _, prior := featureValue(arguments, index, rolloutBudgetPrefillWeightFlag)
	parsed, err := parseFloatFeature(rolloutBudgetPrefillWeightFlag, value, prior)
	if err != nil {
		return index, true, err
	}
	features.prefillWeight = &parsed
	return index + consumed, true, nil
}

func (features *runtimeFeatures) consumeCurrentInterval(arguments []string, index int) (int, bool, error) {
	value, consumed, _, prior := featureValue(arguments, index, currentTimeReminderIntervalFlag)
	parsed, err := parseUintFeature(currentTimeReminderIntervalFlag, value, prior)
	if err != nil {
		return index, true, err
	}
	features.currentInterval = &parsed
	return index + consumed, true, nil
}

func (features *runtimeFeatures) consumeCurrentClock(arguments []string, index int) (int, bool, error) {
	value, consumed, _, err := featureValue(arguments, index, "--current-time-clock-source")
	if err != nil {
		return index, true, err
	}
	if value != "system" && value != "external" {
		return index, true, fmt.Errorf("invalid --current-time-clock-source value %q", value)
	}
	features.currentClock = value
	return index + consumed, true, nil
}

func (features *runtimeFeatures) consumeSystemProxy(arguments []string, index int) (int, bool, error) {
	value := arguments[index] == "--respect-system-proxy"
	if features.respectSystemProxy != nil && *features.respectSystemProxy != value {
		return index, true, errors.New("--respect-system-proxy conflicts with --no-respect-system-proxy")
	}
	features.respectSystemProxy = &value
	return index + 1, true, nil
}

func (features runtimeFeatures) arguments() ([]string, error) {
	if err := features.validateRolloutOptions(); err != nil {
		return nil, err
	}
	overrides := make([]string, 0, 10)
	if features.webSearch != "" {
		overrides = append(overrides, "web_search="+strconv.Quote(features.webSearch))
	}
	overrides = append(overrides, features.rolloutArguments()...)
	overrides = append(overrides, features.currentTimeArguments()...)
	if features.respectSystemProxy != nil {
		overrides = append(overrides, fmt.Sprintf("features.respect_system_proxy=%t", *features.respectSystemProxy))
	}
	return configArguments(overrides), nil
}

func (features runtimeFeatures) validateRolloutOptions() error {
	if features.rolloutLimit == nil && (len(features.rolloutReminders) > 0 || features.samplingWeight != nil || features.prefillWeight != nil) {
		return errors.New("rollout budget options require --rollout-budget-tokens")
	}
	return nil
}

func (features runtimeFeatures) rolloutArguments() []string {
	if features.rolloutLimit == nil || *features.rolloutLimit <= 1 {
		return nil
	}
	limit := *features.rolloutLimit
	overrides := []string{
		"features.rollout_budget.enabled=true",
		fmt.Sprintf("features.rollout_budget.limit_tokens=%d", limit),
		"features.rollout_budget.reminder_at_remaining_tokens=[" + formatUintList(normalizeReminders(limit, features.rolloutReminders)) + "]",
	}
	if features.samplingWeight != nil {
		overrides = append(overrides, "features.rollout_budget.sampling_token_weight="+formatFloat(*features.samplingWeight))
	}
	if features.prefillWeight != nil {
		overrides = append(overrides, "features.rollout_budget.prefill_token_weight="+formatFloat(*features.prefillWeight))
	}
	return overrides
}

func (features runtimeFeatures) currentTimeArguments() []string {
	if !features.currentTime && features.currentInterval == nil && features.currentClock == "" {
		return nil
	}
	overrides := []string{"features.current_time_reminder.enabled=true"}
	if features.currentInterval != nil && *features.currentInterval > 0 {
		overrides = append(overrides, fmt.Sprintf("features.current_time_reminder.reminder_interval_seconds=%d", *features.currentInterval))
	}
	if features.currentClock != "" {
		overrides = append(overrides, "features.current_time_reminder.clock_source="+strconv.Quote(features.currentClock))
	}
	return overrides
}

func configArguments(overrides []string) []string {
	arguments := make([]string, 0, len(overrides)*2)
	for _, override := range overrides {
		arguments = append(arguments, "-c", override)
	}
	return arguments
}

func featureValue(arguments []string, index int, name string) (string, int, bool, error) {
	argument := arguments[index]
	if argument == name {
		if index+1 >= len(arguments) {
			return "", 0, true, fmt.Errorf("%s requires a value", name)
		}
		return arguments[index+1], 2, true, nil
	}
	prefix := name + "="
	if strings.HasPrefix(argument, prefix) {
		value := strings.TrimPrefix(argument, prefix)
		if value == "" {
			return "", 0, true, fmt.Errorf("%s requires a value", name)
		}
		return value, 1, true, nil
	}
	return "", 0, false, nil
}

func parseUintFeature(name, value string, prior error) (uint64, error) {
	if prior != nil {
		return 0, prior
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed > math.MaxInt64 {
		return 0, fmt.Errorf("invalid %s value %q", name, value)
	}
	return parsed, nil
}

func parseUintListFeature(name, value string, prior error) ([]uint64, error) {
	if prior != nil {
		return nil, prior
	}
	parts := strings.Split(value, ",")
	parsed := make([]uint64, 0, len(parts))
	for _, part := range parts {
		item, err := parseUintFeature(name, strings.TrimSpace(part), nil)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, item)
	}
	return parsed, nil
}

func parseFloatFeature(name, value string, prior error) (float64, error) {
	if prior != nil {
		return 0, prior
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("invalid %s value %q", name, value)
	}
	return parsed, nil
}

func normalizeReminders(limit uint64, configured []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(configured)+3)
	values := make([]uint64, 0, len(configured)+3)
	appendBounded := func(value uint64) {
		if value == 0 || value >= limit {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	for _, value := range configured {
		appendBounded(value)
	}
	if len(values) == 0 {
		for _, percent := range []uint64{75, 50, 25} {
			appendBounded(percentOf(limit, percent))
		}
	}
	if len(values) == 0 {
		appendBounded(limit - 1)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] > values[j] })
	return values
}

func percentOf(value, percent uint64) uint64 {
	return value/100*percent + value%100*percent/100
}

func formatUintList(values []uint64) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.FormatUint(value, 10)
	}
	return strings.Join(parts, ",")
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
