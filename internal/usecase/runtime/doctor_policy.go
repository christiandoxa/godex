package runtime

import (
	"fmt"
	goruntime "runtime"
	"strconv"
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type doctorPolicyLaneLimits struct {
	responses, compact, websocket, standard int
}

type doctorPolicyTuning struct {
	activeRequestLimit int
	lanes              doctorPolicyLaneLimits
	admissionWaitMS    uint64
	pressureWaitMS     uint64
	connectWorkers     int
	connectQueue       int
	connectOverflow    int
	dnsWorkers         int
	dnsQueue           int
	dnsOverflow        int
	profileSoft        int
	profileHard        int
}

type doctorPolicySummary struct {
	counts map[string]int
	last   map[string]map[string]string
}

func defaultDoctorPolicyTuning() doctorPolicyTuning {
	parallelism := goruntime.NumCPU()
	workers, longLived := doctorPolicyWorkerCounts(parallelism)
	global := clampDoctorPolicy(workers+longLived*3, 64, 512)
	return doctorPolicyTuning{
		activeRequestLimit: global,
		lanes: doctorPolicyLaneLimits{
			responses: min(global, clampDoctorPolicy(global*3/4, 4, global)),
			compact:   min(global, clampDoctorPolicy(global/4, 2, 6)),
			websocket: min(global, max(longLived, 2)),
			standard:  min(global, clampDoctorPolicy(workers*2, 8, 24)),
		},
		admissionWaitMS: 750, pressureWaitMS: 200,
		connectWorkers: 4, connectQueue: 8, connectOverflow: 16,
		dnsWorkers: 4, dnsQueue: 8, dnsOverflow: 16,
		profileSoft: 4, profileHard: 8,
	}
}

func doctorPolicyWorkerCounts(parallelism int) (int, int) {
	if parallelism <= 0 {
		parallelism = 4
	}
	workers := clampDoctorPolicy(parallelism, 4, 12)
	longLived := 24
	if parallelism < 12 {
		longLived = clampDoctorPolicy(parallelism*2, 8, 24)
	}
	return workers, longLived
}

func clampDoctorPolicy(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func summarizeDoctorPolicyEvents(events []runtimemodel.Event) doctorPolicySummary {
	summary := doctorPolicySummary{counts: make(map[string]int), last: make(map[string]map[string]string)}
	for _, event := range events {
		if !doctorPolicyMarker(event.Kind) {
			continue
		}
		summary.counts[event.Kind]++
		fields := make(map[string]string, len(event.Fields)+4)
		for key, value := range event.Fields {
			fields[key] = value
		}
		if event.AccountID != "" {
			fields["profile"] = event.AccountID
		}
		if event.Path != "" {
			fields["path"] = event.Path
		}
		if event.RequestID != "" {
			fields["request"] = event.RequestID
		}
		summary.last[event.Kind] = fields
	}
	return summary
}

func doctorPolicyMarker(kind string) bool {
	switch kind {
	case "runtime_proxy_lane_limit_reached", "runtime_proxy_active_limit_reached",
		"profile_inflight_saturated", "profile_health",
		"websocket_connect_overflow_rejected", "websocket_connect_overflow_reject",
		"websocket_connect_overflow_enqueue", "websocket_connect_overflow_dispatch",
		"websocket_dns_overflow_reject", "websocket_dns_overflow_enqueue", "websocket_dns_overflow_dispatch",
		"state_save_queue_backpressure", "continuation_journal_queue_backpressure":
		return true
	default:
		return false
	}
}

func doctorPolicySuggestions(events []runtimemodel.Event) []runtimemodel.DoctorPolicySuggestion {
	return doctorPolicySuggestionsWithTuning(summarizeDoctorPolicyEvents(events), defaultDoctorPolicyTuning())
}

func doctorPolicySuggestionsWithTuning(summary doctorPolicySummary, tuning doctorPolicyTuning) []runtimemodel.DoctorPolicySuggestion {
	result := make([]runtimemodel.DoctorPolicySuggestion, 0, 7)
	if count := summary.counts["runtime_proxy_lane_limit_reached"]; count > 0 {
		lane := doctorPolicyField(summary, "runtime_proxy_lane_limit_reached", "lane", "responses")
		key, current := doctorPolicyLaneSetting(tuning, lane)
		if key != "" {
			observedActive := doctorPolicyIntField(summary, "runtime_proxy_lane_limit_reached", "active", current)
			observedLimit := doctorPolicyIntField(summary, "runtime_proxy_lane_limit_reached", "limit", current)
			base := max(current, observedLimit, observedActive+1)
			target := doctorPolicyScaleUp(base)
			settings := []runtimemodel.DoctorPolicySettingSuggestion{doctorPolicySetting(
				key, current, target, fmt.Sprintf("raise the %s lane cap after repeated lane-limit markers", lane),
			)}
			if target >= tuning.activeRequestLimit {
				settings = append(settings, doctorPolicySetting("active_request_limit", tuning.activeRequestLimit, target+2,
					"keep the global admission cap above the suggested lane cap"))
			}
			result = append(result, doctorPolicySuggestion(
				"lane_pressure", "Lane pressure", "medium",
				fmt.Sprintf("%d lane-limit marker(s) on lane=%s; apply only if host/network headroom exists", count, lane),
				[]string{"runtime_proxy_lane_limit_reached"}, settings,
			))
		}
	}
	if count := summary.counts["runtime_proxy_active_limit_reached"]; count > 0 {
		active := doctorPolicyIntField(summary, "runtime_proxy_active_limit_reached", "active", tuning.activeRequestLimit)
		limit := doctorPolicyIntField(summary, "runtime_proxy_active_limit_reached", "limit", tuning.activeRequestLimit)
		base := max(tuning.activeRequestLimit, limit, active+1)
		result = append(result, doctorPolicySuggestion(
			"active_request_pressure", "Active request pressure", "medium",
			fmt.Sprintf("%d global active-limit marker(s); raise only if local CPU/network is not saturated", count),
			[]string{"runtime_proxy_active_limit_reached"},
			[]runtimemodel.DoctorPolicySettingSuggestion{doctorPolicySetting(
				"active_request_limit", tuning.activeRequestLimit, doctorPolicyScaleUp(base),
				"allow more pre-commit requests through local admission",
			)},
		))
	}
	if count := summary.counts["profile_inflight_saturated"]; count > 0 {
		profile := doctorPolicyField(summary, "profile_inflight_saturated", "profile", "unknown")
		hardObserved := doctorPolicyIntField(summary, "profile_inflight_saturated", "hard_limit", tuning.profileHard)
		targetHard := doctorPolicyScaleUp(max(tuning.profileHard, hardObserved))
		targetSoft := doctorPolicyScaleUp(tuning.profileSoft)
		if cap := max(targetHard-1, 1); targetSoft > cap {
			targetSoft = cap
		}
		result = append(result, doctorPolicySuggestion(
			"profile_inflight_saturation", "Profile in-flight saturation", "medium",
			fmt.Sprintf("%d per-profile in-flight saturation marker(s), latest profile=%s; raise only if account fan-out is intentional", count, profile),
			[]string{"profile_inflight_saturated"},
			[]runtimemodel.DoctorPolicySettingSuggestion{
				doctorPolicySetting("profile_inflight_soft_limit", tuning.profileSoft, targetSoft,
					"delay soft load penalty until a profile has more concurrent work"),
				doctorPolicySetting("profile_inflight_hard_limit", tuning.profileHard, targetHard,
					"raise the fresh-selection hard cap for a busy profile"),
			},
		))
	}
	result = append(result, doctorPolicyWebsocketSuggestion(summary, tuning, false)...)
	result = append(result, doctorPolicyWebsocketSuggestion(summary, tuning, true)...)
	stateCount := summary.counts["state_save_queue_backpressure"]
	journalCount := summary.counts["continuation_journal_queue_backpressure"]
	if stateCount+journalCount > 0 {
		targetWait := max(int(tuning.pressureWaitMS), int(tuning.admissionWaitMS)) + 500
		result = append(result, doctorPolicySuggestion(
			"persistence_backpressure", "Persistence backpressure", "medium",
			fmt.Sprintf("state-save backpressure=%d, continuation-journal backpressure=%d; throttle churn while queues drain", stateCount, journalCount),
			[]string{"state_save_queue_backpressure", "continuation_journal_queue_backpressure"},
			[]runtimemodel.DoctorPolicySettingSuggestion{
				doctorPolicySetting("compact_active_limit", tuning.lanes.compact, doctorPolicyScaleDown(tuning.lanes.compact),
					"reduce fresh compact churn that creates continuation state writes"),
				doctorPolicySetting("standard_active_limit", tuning.lanes.standard, doctorPolicyScaleDown(tuning.lanes.standard),
					"reduce side-lane churn while persistence is behind"),
				doctorPolicySetting("pressure_admission_wait_budget_ms", int(tuning.pressureWaitMS), targetWait,
					"let pressure-mode admission wait briefly for queues to drain"),
			},
		))
	}
	if count := summary.counts["profile_health"]; count > 0 {
		profile := doctorPolicyField(summary, "profile_health", "profile", "unknown")
		route := doctorPolicyField(summary, "profile_health", "route", "unknown")
		reason := doctorPolicyField(summary, "profile_health", "reason", "unknown")
		targetSoft := doctorPolicyScaleDown(tuning.profileSoft)
		targetHard := doctorPolicyScaleDown(tuning.profileHard)
		if targetHard < targetSoft+1 {
			targetHard = targetSoft + 1
		}
		result = append(result, doctorPolicySuggestion(
			"route_scoped_profile_health", "Route-scoped profile health", "low",
			fmt.Sprintf("%d route-scoped health marker(s), latest=%s/%s reason=%s; lower per-profile fresh pressure if this repeats", count, profile, route, reason),
			[]string{"profile_health"},
			[]runtimemodel.DoctorPolicySettingSuggestion{
				doctorPolicySetting("profile_inflight_soft_limit", tuning.profileSoft, targetSoft,
					"spread fresh work away from accounts accumulating route-specific health penalties"),
				doctorPolicySetting("profile_inflight_hard_limit", tuning.profileHard, targetHard,
					"cap fresh work per profile more tightly while route health recovers"),
			},
		))
	}
	return result
}

func doctorPolicyWebsocketSuggestion(summary doctorPolicySummary, tuning doctorPolicyTuning, dns bool) []runtimemodel.DoctorPolicySuggestion {
	prefix := "websocket_connect_overflow_"
	id, title := "websocket_connect_overflow", "Websocket connect overflow"
	markers := []string{prefix + "rejected", prefix + "reject", prefix + "enqueue", prefix + "dispatch"}
	workers, queue, overflow := tuning.connectWorkers, tuning.connectQueue, tuning.connectOverflow
	keys := [3]string{"websocket_connect_worker_count", "websocket_connect_queue_capacity", "websocket_connect_overflow_capacity"}
	if dns {
		prefix = "websocket_dns_overflow_"
		id, title = "websocket_dns_overflow", "Websocket DNS overflow"
		markers = []string{prefix + "reject", prefix + "enqueue", prefix + "dispatch"}
		workers, queue, overflow = tuning.dnsWorkers, tuning.dnsQueue, tuning.dnsOverflow
		keys = [3]string{"websocket_dns_worker_count", "websocket_dns_queue_capacity", "websocket_dns_overflow_capacity"}
	}
	count := 0
	for _, marker := range markers {
		count += summary.counts[marker]
	}
	if count == 0 {
		return nil
	}
	latest := "-"
	for _, marker := range markers {
		if summary.counts[marker] > 0 {
			latest = marker
			break
		}
	}
	observedWorkers := doctorPolicyIntField(summary, latest, "worker_count", workers)
	observedQueue := doctorPolicyIntField(summary, latest, "queue_capacity", queue)
	pending := doctorPolicyIntField(summary, latest, "overflow_pending", 0)
	maxPending := doctorPolicyIntField(summary, latest, "overflow_max_pending", 0)
	targetWorkers := doctorPolicyScaleUp(max(workers, observedWorkers))
	targetQueue := doctorPolicyScaleUp(max(queue, observedQueue, targetWorkers))
	targetOverflow := doctorPolicyScaleUp(max(overflow, pending, maxPending, targetQueue))
	return []runtimemodel.DoctorPolicySuggestion{doctorPolicySuggestion(
		id, title, "medium",
		fmt.Sprintf("%d websocket executor overflow marker(s), latest=%s; raise only for bursty session starts", count, latest),
		markers,
		[]runtimemodel.DoctorPolicySettingSuggestion{
			doctorPolicySetting(keys[0], workers, targetWorkers, "increase bounded executor parallelism"),
			doctorPolicySetting(keys[1], queue, targetQueue, "increase bounded executor queue capacity"),
			doctorPolicySetting(keys[2], overflow, targetOverflow, "increase burst overflow buffering after the bounded queue fills"),
		},
	)}
}

func doctorPolicyLaneSetting(tuning doctorPolicyTuning, lane string) (string, int) {
	switch lane {
	case "responses":
		return "responses_active_limit", tuning.lanes.responses
	case "compact":
		return "compact_active_limit", tuning.lanes.compact
	case "websocket":
		return "websocket_active_limit", tuning.lanes.websocket
	case "standard":
		return "standard_active_limit", tuning.lanes.standard
	default:
		return "", 0
	}
}

func doctorPolicySuggestion(id, title, severity, reason string, markers []string, settings []runtimemodel.DoctorPolicySettingSuggestion) runtimemodel.DoctorPolicySuggestion {
	lines := []string{"[runtime_proxy]"}
	for _, setting := range settings {
		lines = append(lines, fmt.Sprintf("%s = %d", setting.Key, setting.SuggestedValue))
	}
	return runtimemodel.DoctorPolicySuggestion{ID: id, Title: title, Severity: severity, Reason: reason, Markers: markers, Settings: settings, Snippet: strings.Join(lines, "\n")}
}

func doctorPolicySetting(key string, current, suggested int, rationale string) runtimemodel.DoctorPolicySettingSuggestion {
	return runtimemodel.DoctorPolicySettingSuggestion{Section: "runtime_proxy", Key: key, CurrentValue: uint64(max(current, 0)), SuggestedValue: uint64(max(suggested, 0)), Rationale: rationale}
}

func doctorPolicyField(summary doctorPolicySummary, marker, field, fallback string) string {
	if fields := summary.last[marker]; fields != nil {
		if value := strings.TrimSpace(fields[field]); value != "" {
			return value
		}
	}
	return fallback
}

func doctorPolicyIntField(summary doctorPolicySummary, marker, field string, fallback int) int {
	value := doctorPolicyField(summary, marker, field, "")
	parsed, err := strconv.ParseUint(value, 10, 31)
	if err != nil {
		return fallback
	}
	return int(parsed)
}

func doctorPolicyScaleUp(value int) int {
	if value < 1 {
		value = 1
	}
	increment := value / 2
	if value%2 == 1 {
		increment++
	}
	return value + increment
}

func doctorPolicyScaleDown(value int) int {
	target := (value*3 + 3) / 4
	if target < 1 {
		return 1
	}
	return target
}
