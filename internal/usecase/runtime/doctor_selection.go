package runtime

import (
	"strings"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type doctorSelectionBucket uint8

const (
	doctorSelectionPicked doctorSelectionBucket = iota + 1
	doctorSelectionKept
	doctorSelectionSkipped
	doctorSelectionBlocked
)

var doctorSelectionBuckets = map[string]doctorSelectionBucket{
	"selection_pick":                     doctorSelectionPicked,
	"selection_keep_affinity":            doctorSelectionKept,
	"selection_keep_current":             doctorSelectionKept,
	"selection_skip_current":             doctorSelectionSkipped,
	"selection_skip_affinity":            doctorSelectionSkipped,
	"selection_skip_sync_probe":          doctorSelectionSkipped,
	"local_selection_blocked":            doctorSelectionBlocked,
	"responses_pre_send_skip":            doctorSelectionBlocked,
	"websocket_pre_send_skip":            doctorSelectionBlocked,
	"quota_critical_floor_before_send":   doctorSelectionBlocked,
	"precommit_budget_exhausted":         doctorSelectionBlocked,
	"compact_precommit_budget_exhausted": doctorSelectionBlocked,
	"compact_candidate_exhausted":        doctorSelectionBlocked,
}

var doctorSelectionDetailFields = []string{
	"profile", "route", "transport", "reason", "status", "code", "exit", "outcome",
	"quota_source", "affinity", "request_shape", "retry_index", "attempt", "response_id",
	"previous_response_id", "session_id",
}

func doctorSelectionSummary(events []runtimemodel.Event) runtimemodel.DoctorSelectionSummary {
	summary := runtimemodel.DoctorSelectionSummary{
		SelectedProfiles: make(map[string]int), SelectedRoutes: make(map[string]int),
		RejectedProfiles: make(map[string]int), RejectedRoutes: make(map[string]int),
		RejectionReasons: make(map[string]int),
	}
	for _, raw := range events {
		event := normalizeDoctorEvent04358(raw)
		bucket, ok := doctorSelectionBuckets[event.Kind]
		if !ok {
			continue
		}
		switch bucket {
		case doctorSelectionPicked:
			summary.Picked++
		case doctorSelectionKept:
			summary.Kept++
		case doctorSelectionSkipped:
			summary.Skipped++
		case doctorSelectionBlocked:
			summary.Blocked++
		}
		selected := bucket == doctorSelectionPicked || bucket == doctorSelectionKept
		profiles, routes := summary.RejectedProfiles, summary.RejectedRoutes
		if selected {
			profiles, routes = summary.SelectedProfiles, summary.SelectedRoutes
		}
		incrementSelectionField(profiles, event.Fields, "profile")
		incrementSelectionField(routes, event.Fields, "route")
		if !selected {
			reason := doctorSelectionReason(event)
			summary.RejectionReasons[reason]++
		}
		detail := doctorSelectionDetail(event.Fields)
		summary.LatestDecision = event.Kind
		if detail != "" {
			summary.LatestDecision += " " + detail
		}
	}
	return summary
}

func incrementSelectionField(counts map[string]int, fields map[string]string, key string) {
	if value := fields[key]; value != "" {
		counts[value]++
	}
}

func doctorSelectionReason(event runtimemodel.Event) string {
	for _, key := range []string{"reason", "outcome", "event", "status"} {
		if value, ok := event.Fields[key]; ok {
			return value
		}
	}
	return event.Kind
}

func doctorSelectionDetail(fields map[string]string) string {
	parts := make([]string, 0, 5)
	for _, key := range doctorSelectionDetailFields {
		value, ok := fields[key]
		if !ok {
			continue
		}
		parts = append(parts, key+"="+truncateSelectionValue(value))
		if len(parts) == 5 {
			break
		}
	}
	return strings.Join(parts, " ")
}

func truncateSelectionValue(value string) string {
	runes := []rune(value)
	if len(runes) <= 48 {
		return value
	}
	return string(runes[:45]) + "..."
}
