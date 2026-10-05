package runtime

import (
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func prodex04355DoctorTestTuning() doctorPolicyTuning {
	return doctorPolicyTuning{
		activeRequestLimit: 8,
		lanes:              doctorPolicyLaneLimits{responses: 6, compact: 1, websocket: 1, standard: 2},
		admissionWaitMS:    0, pressureWaitMS: 250,
		connectWorkers: 4, connectQueue: 8, connectOverflow: 16,
		dnsWorkers: 4, dnsQueue: 8, dnsOverflow: 16,
		profileSoft: 2, profileHard: 4,
	}
}

func TestProdex04355DoctorPolicyLanePressureMatchesPlanner(t *testing.T) {
	summary := summarizeDoctorPolicyEvents([]runtimemodel.Event{
		{Kind: "runtime_proxy_lane_limit_reached", Fields: map[string]string{"lane": "compact", "active": "6", "limit": "6"}},
		{Kind: "runtime_proxy_lane_limit_reached", Fields: map[string]string{"lane": "compact", "active": "7", "limit": "6"}},
	})
	suggestions := doctorPolicySuggestionsWithTuning(summary, prodex04355DoctorTestTuning())
	if len(suggestions) != 1 {
		t.Fatalf("suggestions = %#v", suggestions)
	}
	got := suggestions[0]
	if got.ID != "lane_pressure" || got.Title != "Lane pressure" || got.Severity != "medium" ||
		got.Reason != "2 lane-limit marker(s) on lane=compact; apply only if host/network headroom exists" ||
		got.Snippet != "[runtime_proxy]\ncompact_active_limit = 12\nactive_request_limit = 14" || len(got.Settings) != 2 ||
		got.Settings[0].CurrentValue != 1 || got.Settings[0].SuggestedValue != 12 ||
		got.Settings[1].CurrentValue != 8 || got.Settings[1].SuggestedValue != 14 {
		t.Fatalf("lane suggestion = %#v", got)
	}
}

func TestProdex04355DoctorPolicyActiveAndDNSPressureMatchPlanner(t *testing.T) {
	tuning := prodex04355DoctorTestTuning()
	active := doctorPolicySuggestionsWithTuning(summarizeDoctorPolicyEvents([]runtimemodel.Event{
		{Kind: "runtime_proxy_active_limit_reached", Fields: map[string]string{"active": "64", "limit": "64"}},
		{Kind: "runtime_proxy_active_limit_reached", Fields: map[string]string{"active": "65", "limit": "64"}},
	}), tuning)
	if len(active) != 1 || active[0].ID != "active_request_pressure" || active[0].Settings[0].SuggestedValue != 99 {
		t.Fatalf("active suggestion = %#v", active)
	}
	dns := doctorPolicySuggestionsWithTuning(summarizeDoctorPolicyEvents([]runtimemodel.Event{
		{Kind: "websocket_dns_overflow_enqueue", Fields: map[string]string{"worker_count": "2", "queue_capacity": "8", "overflow_pending": "1", "overflow_max_pending": "1"}},
		{Kind: "websocket_dns_overflow_reject", Fields: map[string]string{"worker_count": "2", "queue_capacity": "8", "overflow_pending": "3", "overflow_max_pending": "3"}},
	}), tuning)
	if len(dns) != 1 || dns[0].ID != "websocket_dns_overflow" || dns[0].Settings[0].SuggestedValue != 6 ||
		dns[0].Settings[1].SuggestedValue != 12 || dns[0].Settings[2].SuggestedValue != 24 {
		t.Fatalf("DNS suggestion = %#v", dns)
	}
}

func TestProdex04355DoctorPolicyPressureSuggestionsMatchPlanner(t *testing.T) {
	tuning := prodex04355DoctorTestTuning()
	tests := []struct {
		name   string
		events []runtimemodel.Event
		id     string
		check  func(*testing.T, runtimemodel.DoctorPolicySuggestion)
	}{
		{
			name: "profile inflight",
			events: []runtimemodel.Event{
				{Kind: "profile_inflight_saturated", Fields: map[string]string{"profile": "main", "hard_limit": "8", "route": "responses"}},
				{Kind: "profile_inflight_saturated", Fields: map[string]string{"profile": "main", "hard_limit": "8", "route": "websocket"}},
			},
			id: "profile_inflight_saturation",
			check: func(t *testing.T, got runtimemodel.DoctorPolicySuggestion) {
				if got.Settings[0].SuggestedValue != 3 || got.Settings[1].SuggestedValue != 12 ||
					got.Reason != "2 per-profile in-flight saturation marker(s), latest profile=main; raise only if account fan-out is intentional" {
					t.Fatalf("profile suggestion = %#v", got)
				}
			},
		},
		{
			name:   "persistence",
			events: []runtimemodel.Event{{Kind: "state_save_queue_backpressure"}, {Kind: "continuation_journal_queue_backpressure"}},
			id:     "persistence_backpressure",
			check: func(t *testing.T, got runtimemodel.DoctorPolicySuggestion) {
				if got.Settings[0].SuggestedValue != 1 || got.Settings[1].SuggestedValue != 2 || got.Settings[2].SuggestedValue != 750 {
					t.Fatalf("persistence suggestion = %#v", got)
				}
			},
		},
		{
			name:   "route health",
			events: []runtimemodel.Event{{Kind: "profile_health", Fields: map[string]string{"profile": "alpha", "route": "responses", "reason": "stream_read_error", "score": "43"}}},
			id:     "route_scoped_profile_health",
			check: func(t *testing.T, got runtimemodel.DoctorPolicySuggestion) {
				if got.Severity != "low" || got.Settings[0].SuggestedValue != 2 || got.Settings[1].SuggestedValue != 3 ||
					got.Reason != "1 route-scoped health marker(s), latest=alpha/responses reason=stream_read_error; lower per-profile fresh pressure if this repeats" {
					t.Fatalf("health suggestion = %#v", got)
				}
			},
		},
		{
			name: "websocket connect",
			events: []runtimemodel.Event{
				{Kind: "websocket_connect_overflow_enqueue", Fields: map[string]string{"worker_count": "1", "queue_capacity": "1", "overflow_pending": "1", "overflow_max_pending": "1"}},
				{Kind: "websocket_connect_overflow_reject", Fields: map[string]string{"worker_count": "1", "queue_capacity": "1", "overflow_pending": "3", "overflow_max_pending": "3"}},
			},
			id: "websocket_connect_overflow",
			check: func(t *testing.T, got runtimemodel.DoctorPolicySuggestion) {
				if got.Settings[0].SuggestedValue != 6 || got.Settings[1].SuggestedValue != 12 || got.Settings[2].SuggestedValue != 24 {
					t.Fatalf("connect suggestion = %#v", got)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suggestions := doctorPolicySuggestionsWithTuning(summarizeDoctorPolicyEvents(test.events), tuning)
			if len(suggestions) != 1 || suggestions[0].ID != test.id {
				t.Fatalf("suggestions = %#v", suggestions)
			}
			test.check(t, suggestions[0])
		})
	}
}
