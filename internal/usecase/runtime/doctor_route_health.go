package runtime

import (
	"sort"
	"strconv"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const doctorRouteProfileEventLimit = 20

var doctorRouteProfileMarkers = map[string]struct{}{
	"selection_keep_affinity": {}, "selection_keep_current": {}, "selection_pick": {},
	"selection_skip_current": {}, "selection_skip_affinity": {}, "selection_skip_sync_probe": {},
	"local_selection_blocked": {}, "responses_pre_send_skip": {}, "websocket_pre_send_skip": {},
	"quota_critical_floor_before_send": {}, "profile_health": {}, "profile_latency": {},
	"profile_bad_pairing": {}, "profile_transport_backoff": {}, "profile_transport_failure": {},
	"profile_circuit_open": {}, "profile_circuit_half_open_probe": {}, "stream_read_error": {},
	"upstream_connect_timeout": {}, "upstream_connect_dns_error": {}, "upstream_tls_handshake_error": {},
	"upstream_connect_error": {}, "upstream_connect_http": {}, "upstream_close_before_completed": {},
	"upstream_connection_closed": {}, "upstream_read_error": {}, "upstream_send_error": {},
	"upstream_stream_error": {}, "compact_transport_failure": {}, "websocket_precommit_frame_timeout": {},
	"websocket_precommit_hold_timeout": {}, "quota_blocked": {}, "profile_retry_backoff": {},
	"profile_quota_quarantine": {}, "compact_retryable_failure": {}, "compact_quota_unclassified": {},
}

type doctorRouteHealthKey struct{ profile, route string }

func doctorRouteHealth(events []runtimemodel.Event) []runtimemodel.DoctorRouteHealth {
	return doctorRouteHealthWithState(events, nil)
}

type doctorRouteHealthState struct {
	now      time.Time
	health   []routingentity.RouteHealthScore
	memory   []routingentity.RouteMemoryScore
	circuits []routingentity.RouteCircuit
	backoffs []routingentity.TransportBackoff
}

func doctorRouteHealthWithState(events []runtimemodel.Event, state *doctorRouteHealthState) []runtimemodel.DoctorRouteHealth {
	selected := make([]runtimemodel.Event, 0, doctorRouteProfileEventLimit)
	for _, event := range events {
		event = normalizeDoctorEvent04358(event)
		if _, ok := doctorRouteProfileMarkers[event.Kind]; !ok {
			continue
		}
		if event.Fields["profile"] == "" && event.Fields["route"] == "" {
			continue
		}
		if len(selected) == doctorRouteProfileEventLimit {
			copy(selected, selected[1:])
			selected[len(selected)-1] = event
			continue
		}
		selected = append(selected, event)
	}
	groups := make(map[doctorRouteHealthKey]*runtimemodel.DoctorRouteHealth, len(selected))
	if state != nil {
		doctorApplyPersistedRouteHealth(groups, *state)
	}
	for _, event := range selected {
		profile := event.Fields["profile"]
		if profile == "" {
			profile = "unknown"
		}
		route := event.Fields["route"]
		if route == "" {
			route = "unknown"
		}
		key := doctorRouteHealthKey{profile: profile, route: route}
		entry := groups[key]
		if entry == nil {
			entry = &runtimemodel.DoctorRouteHealth{Profile: profile, Route: route, HealthScore: nil, BadPairingScore: nil, LatencyScore: nil}
			groups[key] = entry
		}
		entry.EventCount++
		entry.LatestMarker = event.Kind
		if reason := event.Fields["reason"]; reason != "" {
			entry.LatestReason = reason
		}
		if event.TimestampUnixMilli > 0 {
			entry.LatestTimestamp = time.UnixMilli(event.TimestampUnixMilli).UTC().Format(time.RFC3339Nano)
		}
		doctorApplyRouteHealthMarker(entry, event)
	}
	result := make([]runtimemodel.DoctorRouteHealth, 0, len(groups))
	for _, entry := range groups {
		result = append(result, *entry)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].EventCount != result[right].EventCount {
			return result[left].EventCount > result[right].EventCount
		}
		if result[left].Profile != result[right].Profile {
			return result[left].Profile < result[right].Profile
		}
		return result[left].Route < result[right].Route
	})
	return result
}

func doctorApplyPersistedRouteHealth(
	groups map[doctorRouteHealthKey]*runtimemodel.DoctorRouteHealth,
	state doctorRouteHealthState,
) {
	entryFor := func(profile, route string) *runtimemodel.DoctorRouteHealth {
		key := doctorRouteHealthKey{profile: profile, route: route}
		entry := groups[key]
		if entry == nil {
			entry = &runtimemodel.DoctorRouteHealth{Profile: profile, Route: route}
			groups[key] = entry
		}
		return entry
	}
	for _, score := range state.health {
		entry := entryFor(score.AccountID, score.Route)
		value := uint32(score.Effective(state.now))
		entry.HealthScore = &value
	}
	for _, score := range state.memory {
		entry := entryFor(score.AccountID, score.Route)
		value := uint32(score.Effective(state.now))
		switch score.Kind {
		case routingentity.RouteMemoryBadPairing:
			entry.BadPairingScore = &value
		case routingentity.RouteMemoryPerformance:
			entry.LatencyScore = &value
		}
	}
	for _, circuit := range state.circuits {
		entry := entryFor(circuit.AccountID, circuit.Route)
		entry.CircuitState = "open"
		until := circuit.UntilUnix
		entry.CircuitUntil = &until
	}
	for _, backoff := range state.backoffs {
		entry := entryFor(backoff.AccountID, backoff.Route)
		until := backoff.UntilUnix
		entry.TransportBackoffUntil = &until
	}
}

func doctorApplyRouteHealthMarker(entry *runtimemodel.DoctorRouteHealth, event runtimemodel.Event) {
	score := func() *uint32 {
		value, err := strconv.ParseUint(event.Fields["score"], 10, 32)
		if err != nil {
			return nil
		}
		converted := uint32(value)
		return &converted
	}
	until := func() *int64 {
		value, err := strconv.ParseInt(event.Fields["until"], 10, 64)
		if err != nil {
			return nil
		}
		return &value
	}
	switch event.Kind {
	case "profile_health":
		if value := score(); value != nil {
			entry.HealthScore = value
		}
		if reason := event.Fields["reason"]; reason != "" {
			entry.HealthReason = reason
		}
	case "profile_bad_pairing":
		if value := score(); value != nil {
			entry.BadPairingScore = value
		}
		if reason := event.Fields["reason"]; reason != "" {
			entry.BadPairingReason = reason
		}
	case "profile_latency":
		if value := score(); value != nil {
			entry.LatencyScore = value
		}
		if reason := event.Fields["reason"]; reason != "" {
			entry.LatencyReason = reason
		}
	case "profile_circuit_open":
		entry.CircuitState = "open"
		if value := until(); value != nil {
			entry.CircuitUntil = value
		}
		if reason := event.Fields["reason"]; reason != "" {
			entry.CircuitReason = reason
		}
	case "profile_transport_backoff":
		if value := until(); value != nil {
			entry.TransportBackoffUntil = value
		}
		if context := event.Fields["context"]; context != "" {
			entry.TransportBackoffContext = context
		} else if reason := event.Fields["reason"]; reason != "" {
			entry.TransportBackoffContext = reason
		}
	}
}
