package runtime

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type runtimeFeatures struct {
	webSearch          string
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
	argument := arguments[index]
	if value, consumed, ok, valueErr := featureValue(arguments, index, "--web-search"); ok {
		if valueErr != nil {
			return index, true, valueErr
		}
		switch value {
		case "disabled", "cached", "indexed", "live":
			features.webSearch = value
			return index + consumed, true, nil
		default:
			return index, true, fmt.Errorf("invalid --web-search value %q", value)
		}
	}
	if value, consumed, ok, valueErr := featureValue(arguments, index, "--rollout-budget-tokens"); ok {
		parsed, parseErr := parseUintFeature("--rollout-budget-tokens", value, valueErr)
		if parseErr != nil {
			return index, true, parseErr
		}
		features.rolloutLimit = &parsed
		return index + consumed, true, nil
	}
	if value, consumed, ok, valueErr := featureValue(arguments, index, "--rollout-budget-reminders"); ok {
		parsed, parseErr := parseUintListFeature("--rollout-budget-reminders", value, valueErr)
		if parseErr != nil {
			return index, true, parseErr
		}
		features.rolloutReminders = parsed
		return index + consumed, true, nil
	}
	if value, consumed, ok, valueErr := featureValue(arguments, index, "--rollout-budget-sampling-weight"); ok {
		parsed, parseErr := parseFloatFeature("--rollout-budget-sampling-weight", value, valueErr)
		if parseErr != nil {
			return index, true, parseErr
		}
		features.samplingWeight = &parsed
		return index + consumed, true, nil
	}
	if value, consumed, ok, valueErr := featureValue(arguments, index, "--rollout-budget-prefill-weight"); ok {
		parsed, parseErr := parseFloatFeature("--rollout-budget-prefill-weight", value, valueErr)
		if parseErr != nil {
			return index, true, parseErr
		}
		features.prefillWeight = &parsed
		return index + consumed, true, nil
	}
	if argument == "--current-time-reminder" {
		features.currentTime = true
		return index + 1, true, nil
	}
	if value, consumed, ok, valueErr := featureValue(arguments, index, "--current-time-reminder-interval"); ok {
		parsed, parseErr := parseUintFeature("--current-time-reminder-interval", value, valueErr)
		if parseErr != nil {
			return index, true, parseErr
		}
		features.currentInterval = &parsed
		return index + consumed, true, nil
	}
	if value, consumed, ok, valueErr := featureValue(arguments, index, "--current-time-clock-source"); ok {
		if valueErr != nil {
			return index, true, valueErr
		}
		if value != "system" && value != "external" {
			return index, true, fmt.Errorf("invalid --current-time-clock-source value %q", value)
		}
		features.currentClock = value
		return index + consumed, true, nil
	}
	if argument == "--respect-system-proxy" || argument == "--no-respect-system-proxy" {
		value := argument == "--respect-system-proxy"
		if features.respectSystemProxy != nil && *features.respectSystemProxy != value {
			return index, true, errors.New("--respect-system-proxy conflicts with --no-respect-system-proxy")
		}
		features.respectSystemProxy = &value
		return index + 1, true, nil
	}
	return index, false, nil
}

func (features runtimeFeatures) arguments() ([]string, error) {
	if features.rolloutLimit == nil && (len(features.rolloutReminders) > 0 || features.samplingWeight != nil || features.prefillWeight != nil) {
		return nil, errors.New("rollout budget options require --rollout-budget-tokens")
	}
	overrides := make([]string, 0, 10)
	if features.webSearch != "" {
		overrides = append(overrides, "web_search="+strconv.Quote(features.webSearch))
	}
	if features.rolloutLimit != nil && *features.rolloutLimit > 1 {
		limit := *features.rolloutLimit
		overrides = append(overrides,
			"features.rollout_budget.enabled=true",
			fmt.Sprintf("features.rollout_budget.limit_tokens=%d", limit),
			"features.rollout_budget.reminder_at_remaining_tokens=["+formatUintList(normalizeReminders(limit, features.rolloutReminders))+"]",
		)
		if features.samplingWeight != nil {
			overrides = append(overrides, "features.rollout_budget.sampling_token_weight="+formatFloat(*features.samplingWeight))
		}
		if features.prefillWeight != nil {
			overrides = append(overrides, "features.rollout_budget.prefill_token_weight="+formatFloat(*features.prefillWeight))
		}
	}
	if features.currentTime || features.currentInterval != nil || features.currentClock != "" {
		overrides = append(overrides, "features.current_time_reminder.enabled=true")
		if features.currentInterval != nil && *features.currentInterval > 0 {
			overrides = append(overrides, fmt.Sprintf("features.current_time_reminder.reminder_interval_seconds=%d", *features.currentInterval))
		}
		if features.currentClock != "" {
			overrides = append(overrides, "features.current_time_reminder.clock_source="+strconv.Quote(features.currentClock))
		}
	}
	if features.respectSystemProxy != nil {
		overrides = append(overrides, fmt.Sprintf("features.respect_system_proxy=%t", *features.respectSystemProxy))
	}
	arguments := make([]string, 0, len(overrides)*2)
	for _, override := range overrides {
		arguments = append(arguments, "-c", override)
	}
	return arguments, nil
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
