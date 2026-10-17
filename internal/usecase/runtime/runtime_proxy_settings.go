package runtime

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type runtimeProxySettings struct {
	activeRequestLimit               int
	profileInflightHardLimit         int
	admissionWaitBudget              time.Duration
	pressureAdmissionWaitBudget      time.Duration
	longLivedQueueWaitBudget         time.Duration
	pressureLongLivedQueueWaitBudget time.Duration
}

func defaultRuntimeProxySettings() runtimeProxySettings {
	return runtimeProxySettings{
		admissionWaitBudget:              750 * time.Millisecond,
		pressureAdmissionWaitBudget:      200 * time.Millisecond,
		longLivedQueueWaitBudget:         750 * time.Millisecond,
		pressureLongLivedQueueWaitBudget: 200 * time.Millisecond,
	}
}

func runtimeProxySettingsFromEnv() (runtimeProxySettings, error) {
	return runtimeProxySettingsWithLookup(os.LookupEnv)
}

func runtimeProxySettingsWithLookup(lookup func(string) (string, bool)) (runtimeProxySettings, error) {
	settings := defaultRuntimeProxySettings()
	var err error
	if settings.activeRequestLimit, err = positiveRuntimeInt(lookup, "GODEX_RUNTIME_PROXY_ACTIVE_REQUEST_LIMIT", "PRODEX_RUNTIME_PROXY_ACTIVE_REQUEST_LIMIT"); err != nil {
		return runtimeProxySettings{}, err
	}
	if settings.profileInflightHardLimit, err = positiveRuntimeInt(lookup,
		"GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT",
		"PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT"); err != nil {
		return runtimeProxySettings{}, err
	}
	if settings.admissionWaitBudget, err = positiveRuntimeDuration(lookup, "GODEX_RUNTIME_PROXY_ADMISSION_WAIT_BUDGET_MS", "PRODEX_RUNTIME_PROXY_ADMISSION_WAIT_BUDGET_MS", settings.admissionWaitBudget); err != nil {
		return runtimeProxySettings{}, err
	}
	if settings.pressureAdmissionWaitBudget, err = positiveRuntimeDuration(lookup, "GODEX_RUNTIME_PROXY_PRESSURE_ADMISSION_WAIT_BUDGET_MS", "PRODEX_RUNTIME_PROXY_PRESSURE_ADMISSION_WAIT_BUDGET_MS", settings.pressureAdmissionWaitBudget); err != nil {
		return runtimeProxySettings{}, err
	}
	if settings.longLivedQueueWaitBudget, err = positiveRuntimeDuration(lookup, "GODEX_RUNTIME_PROXY_LONG_LIVED_QUEUE_WAIT_BUDGET_MS", "PRODEX_RUNTIME_PROXY_LONG_LIVED_QUEUE_WAIT_BUDGET_MS", settings.longLivedQueueWaitBudget); err != nil {
		return runtimeProxySettings{}, err
	}
	if settings.pressureLongLivedQueueWaitBudget, err = positiveRuntimeDuration(lookup, "GODEX_RUNTIME_PROXY_PRESSURE_LONG_LIVED_QUEUE_WAIT_BUDGET_MS", "PRODEX_RUNTIME_PROXY_PRESSURE_LONG_LIVED_QUEUE_WAIT_BUDGET_MS", settings.pressureLongLivedQueueWaitBudget); err != nil {
		return runtimeProxySettings{}, err
	}
	return settings, nil
}

func positiveRuntimeInt(lookup func(string) (string, bool), canonical, legacy string) (int, error) {
	value, found := runtimeProxyEnvValue(lookup, canonical, legacy)
	if !found {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", canonical)
	}
	if parsed == 0 {
		return 0, fmt.Errorf("%s must be greater than zero", canonical)
	}
	maxInt := uint64(^uint(0) >> 1)
	if parsed > maxInt {
		return 0, fmt.Errorf("%s exceeds the supported limit", canonical)
	}
	return int(parsed), nil
}

func positiveRuntimeDuration(
	lookup func(string) (string, bool), canonical, legacy string, fallback time.Duration,
) (time.Duration, error) {
	value, found := runtimeProxyEnvValue(lookup, canonical, legacy)
	if !found {
		return fallback, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 63)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", canonical)
	}
	if parsed == 0 {
		return 0, fmt.Errorf("%s must be greater than zero", canonical)
	}
	if parsed > uint64((1<<63-1)/int64(time.Millisecond)) {
		return 0, fmt.Errorf("%s exceeds the supported duration", canonical)
	}
	return time.Duration(parsed) * time.Millisecond, nil
}

func runtimeProxyEnvValue(lookup func(string) (string, bool), canonical, legacy string) (string, bool) {
	if value, found := lookup(canonical); found {
		return value, true
	}
	return lookup(legacy)
}
