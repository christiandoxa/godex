package runtime

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/christiandoxa/godex/internal/helper/redact"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

// doctorKnownRuntimeMarkers04358 is the exact Prodex 0.435.8 runtime-doctor marker vocabulary.
var doctorKnownRuntimeMarkers04358 = map[string]struct{}{
	"chain_retried_owner":                                {},
	"chain_dead_upstream_confirmed":                      {},
	"stale_continuation":                                 {},
	"runtime_proxy_queue_overloaded":                     {},
	"runtime_proxy_active_limit_reached":                 {},
	"runtime_proxy_lane_limit_reached":                   {},
	"runtime_proxy_overload_backoff":                     {},
	"runtime_proxy_admission_wait_started":               {},
	"runtime_proxy_admission_wait_exhausted":             {},
	"runtime_proxy_admission_recovered":                  {},
	"runtime_proxy_queue_wait_started":                   {},
	"runtime_proxy_queue_wait_exhausted":                 {},
	"runtime_proxy_queue_recovered":                      {},
	"profile_inflight_saturated":                         {},
	"profile_inflight":                                   {},
	"upstream_connect_timeout":                           {},
	"upstream_connect_dns_error":                         {},
	"upstream_tls_handshake_error":                       {},
	"upstream_connect_error":                             {},
	"upstream_connect_http":                              {},
	"upstream_close_before_completed":                    {},
	"upstream_connection_closed":                         {},
	"upstream_overload_passthrough":                      {},
	"upstream_overloaded":                                {},
	"upstream_read_error":                                {},
	"upstream_send_error":                                {},
	"upstream_stream_error":                              {},
	"precommit_budget_exhausted":                         {},
	"profile_retry_backoff":                              {},
	"profile_transport_backoff":                          {},
	"profile_transport_failure":                          {},
	"profile_circuit_open":                               {},
	"profile_circuit_half_open_probe":                    {},
	"profile_health":                                     {},
	"profile_latency":                                    {},
	"profile_bad_pairing":                                {},
	"profile_quota_quarantine":                           {},
	"profile_auth_backoff":                               {},
	"profile_auth_backoff_cleared":                       {},
	"profile_auth_proactive_sync":                        {},
	"profile_auth_proactive_sync_failed":                 {},
	"previous_response_not_found":                        {},
	"previous_response_negative_cache":                   {},
	"previous_response_fresh_fallback":                   {},
	"previous_response_fresh_fallback_blocked":           {},
	"previous_response_binding_cleared":                  {},
	"previous_response_owner":                            {},
	"previous_response_release_affinity":                 {},
	"previous_response_release_deferred":                 {},
	"previous_response_turn_state_rehydrated":            {},
	"compact_committed_owner":                            {},
	"compact_followup_owner":                             {},
	"compact_fresh_fallback_blocked":                     {},
	"compact_pressure_shed":                              {},
	"compact_lineage_released":                           {},
	"compact_committed":                                  {},
	"compact_precommit_budget_exhausted":                 {},
	"compact_candidate_exhausted":                        {},
	"compact_retryable_failure":                          {},
	"compact_transport_failure":                          {},
	"compact_overload_conservative_retry":                {},
	"compact_quota_unclassified":                         {},
	"compact_pre_send_allow_quota_exhausted":             {},
	"compact_final_failure":                              {},
	"compact_exit_committed":                             {},
	"compact_exit_committed_owner":                       {},
	"compact_exit_followup_owner":                        {},
	"compact_exit_fresh_fallback_blocked":                {},
	"compact_exit_pressure_shed":                         {},
	"compact_exit_lineage_released":                      {},
	"compact_exit_precommit_budget_exhausted":            {},
	"compact_exit_candidate_exhausted":                   {},
	"compact_exit_retryable_failure":                     {},
	"compact_exit_overload_conservative_retry":           {},
	"compact_exit_quota_unclassified":                    {},
	"selection_keep_affinity":                            {},
	"selection_keep_current":                             {},
	"selection_plan":                                     {},
	"selection_pick":                                     {},
	"selection_skip_current":                             {},
	"selection_skip_affinity":                            {},
	"local_selection_blocked":                            {},
	"responses_pre_send_skip":                            {},
	"websocket_pre_send_skip":                            {},
	"quota_release_profile_affinity":                     {},
	"quota_release_affinity":                             {},
	"quota_blocked":                                      {},
	"quota_critical_floor_before_send":                   {},
	"upstream_usage_limit_passthrough":                   {},
	"local_rewrite_upstream_start":                       {},
	"local_rewrite_upstream_response":                    {},
	"local_rewrite_request_detail":                       {},
	"local_rewrite_web_search_options_fallback":          {},
	"local_rewrite_provider_model_fallback":              {},
	"local_rewrite_provider_auth_failure":                {},
	"local_rewrite_gemini_builtin_tool_fallback":         {},
	"local_rewrite_gemini_quota_rotate":                  {},
	"local_rewrite_gemini_rate_limit_retry":              {},
	"local_rewrite_gemini_invalid_stream_retry":          {},
	"local_rewrite_gemini_invalid_stream_model_fallback": {},
	"local_rewrite_gemini_quota_status_ready":            {},
	"local_rewrite_gemini_quota_status_unavailable":      {},
	"local_rewrite_gemini_compact_semantic":              {},
	"local_rewrite_gemini_compact_fallback":              {},
	"local_rewrite_gemini_synthetic_thought_signature":   {},
	"local_rewrite_gemini_live_sidecar_started":          {},
	"local_rewrite_gemini_live_sidecar_error":            {},
	"local_rewrite_gemini_live_sidecar_accept_error":     {},
	"local_rewrite_gemini_live_connected":                {},
	"local_rewrite_gemini_live_error":                    {},
	"local_rewrite_gemini_live_sidecar_connected":        {},
	"local_rewrite_gemini_live_sidecar_session_error":    {},
	"local_rewrite_gemini_live_frame":                    {},
	"local_rewrite_gemini_live_duplex_pump":              {},
	"compat_request_surface":                             {},
	"compat_warning":                                     {},
	"smart_context_autopilot":                            {},
	"runtime_proxy_sync_probe_pressure_pause":            {},
	"websocket_reuse_skip_quota_exhausted":               {},
	"websocket_reuse_watchdog":                           {},
	"websocket_reuse_watchdog_timeout":                   {},
	"websocket_reuse_locked_affinity_owner_fresh_retry":  {},
	"websocket_reuse_nonreplayable_fresh_retry":          {},
	"websocket_reuse_owner_fresh_retry":                  {},
	"websocket_reuse_previous_response_blocked":          {},
	"websocket_reuse_stale_previous_response_blocked":    {},
	"websocket_precommit_frame_timeout":                  {},
	"websocket_precommit_hold_timeout":                   {},
	"websocket_dns_resolve_timeout":                      {},
	"websocket_dns_overflow_enqueue":                     {},
	"websocket_dns_overflow_dispatch":                    {},
	"websocket_dns_overflow_reject":                      {},
	"websocket_connect_local_pressure":                   {},
	"websocket_connect_overflow_enqueue":                 {},
	"websocket_connect_overflow_dispatch":                {},
	"websocket_connect_overflow_reject":                  {},
	"websocket_connect_overflow_rejected":                {},
	"websocket_proxy_connect_start":                      {},
	"websocket_proxy_tunnel_ok":                          {},
	"websocket_proxy_tunnel_failure":                     {},
	"profile_auth_recovered":                             {},
	"profile_auth_recovery_failed":                       {},
	"stream_read_error":                                  {},
	"token_usage":                                        {},
	"local_writer_error":                                 {},
	"first_upstream_chunk":                               {},
	"first_local_chunk":                                  {},
	"state_save_ok":                                      {},
	"state_save_skipped":                                 {},
	"state_save_error":                                   {},
	"state_save_queued":                                  {},
	"state_save_queue_backpressure":                      {},
	"continuation_journal_save_ok":                       {},
	"continuation_journal_save_error":                    {},
	"continuation_journal_save_queued":                   {},
	"continuation_journal_queue_backpressure":            {},
	"runtime_proxy_restore_counts":                       {},
	"runtime_proxy_startup_audit":                        {},
	"runtime_proxy_upstream_proxy_mode":                  {},
	"profile_probe_refresh_queued":                       {},
	"profile_probe_refresh_start":                        {},
	"profile_probe_refresh_ok":                           {},
	"profile_probe_refresh_error":                        {},
	"profile_probe_refresh_backpressure":                 {},
	"profile_probe_refresh_panic":                        {},
	"selection_skip_sync_probe":                          {},
	"quota_blocked_affinity_released":                    {},
}

func normalizeDoctorEvent04358(event runtimemodel.Event) runtimemodel.Event {
	message := doctorMessageBody04358(event.Message)
	if _, known := doctorKnownRuntimeMarkers04358[event.Kind]; !known {
		if marker := doctorMarkerFromMessage04358(message); marker != "" {
			event.Kind = marker
		}
	}
	parsed := doctorMessageFields04358(message)
	if len(parsed) == 0 && len(event.Fields) == 0 {
		return event
	}
	merged := make(map[string]string, len(parsed)+len(event.Fields))
	for key, value := range parsed {
		merged[key] = value
	}
	for key, value := range event.Fields {
		merged[key] = sanitizeDoctorField04358(key, value)
	}
	event.Fields = merged
	return event
}

func doctorMessageBody04358(message string) string {
	message = strings.TrimSpace(message)
	if strings.HasPrefix(message, "[") {
		if end := strings.Index(message, "] "); end >= 0 {
			return strings.TrimSpace(message[end+2:])
		}
	}
	return message
}

func doctorMarkerFromMessage04358(message string) string {
	for index := 0; index < len(message); {
		index = skipDoctorSpaces04358(message, index)
		if index >= len(message) {
			break
		}
		start := index
		index = skipDoctorKeyOrToken04358(message, index)
		if index < len(message) && message[index] == '=' {
			index = skipDoctorValue04358(message, index+1)
			continue
		}
		if start < index {
			candidate := message[start:index]
			if _, ok := doctorKnownRuntimeMarkers04358[candidate]; ok {
				return candidate
			}
			break
		}
		index++
	}
	for index := 0; index < len(message); {
		for index < len(message) && !doctorMarkerTokenByte04358(message[index]) {
			index++
		}
		start := index
		for index < len(message) && doctorMarkerTokenByte04358(message[index]) {
			index++
		}
		if start < index {
			candidate := message[start:index]
			if _, ok := doctorKnownRuntimeMarkers04358[candidate]; ok {
				return candidate
			}
		}
	}
	return ""
}

func doctorMessageFields04358(message string) map[string]string {
	fields := map[string]string{}
	for index := 0; index < len(message); {
		index = skipDoctorSpaces04358(message, index)
		if index >= len(message) {
			break
		}
		keyStart := index
		index = skipDoctorKeyOrToken04358(message, index)
		if keyStart == index || index >= len(message) || message[index] != '=' {
			index = skipDoctorBareToken04358(message, index)
			continue
		}
		key := message[keyStart:index]
		valueStart := index + 1
		valueEnd := skipDoctorValue04358(message, valueStart)
		if valueEnd > valueStart {
			raw := message[valueStart:valueEnd]
			value := raw
			if strings.HasPrefix(raw, "\"") {
				var decoded string
				if json.Unmarshal([]byte(raw), &decoded) == nil {
					value = decoded
				}
			}
			fields[key] = sanitizeDoctorField04358(key, value)
		}
		index = valueEnd
	}
	return fields
}

func sanitizeDoctorField04358(key, value string) string {
	lower := strings.ToLower(strings.TrimSpace(key))
	switch lower {
	case "authorization", "proxy-authorization", "api_key", "openai_api_key", "anthropic_api_key",
		"gemini_api_key", "google_api_key", "github_copilot_api_key", "access_token",
		"refresh_token", "id_token", "password", "secret", "cookie", "set-cookie":
		return "<redacted>"
	}
	value = redact.Secrets(value)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

func skipDoctorSpaces04358(message string, index int) int {
	for index < len(message) && (message[index] == ' ' || message[index] == '\t' || message[index] == '\r' || message[index] == '\n') {
		index++
	}
	return index
}

func skipDoctorKeyOrToken04358(message string, index int) int {
	for index < len(message) {
		value := message[index]
		if value == '=' || value == ' ' || value == '\t' || value == '\r' || value == '\n' {
			break
		}
		index++
	}
	return index
}

func skipDoctorBareToken04358(message string, index int) int {
	for index < len(message) {
		value := message[index]
		if value == ' ' || value == '\t' || value == '\r' || value == '\n' {
			break
		}
		index++
	}
	return index
}

func skipDoctorValue04358(message string, index int) int {
	if index < len(message) && message[index] == '"' {
		index++
		escaped := false
		for index < len(message) {
			value := message[index]
			index++
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
				continue
			}
			if value == '"' {
				break
			}
		}
		return index
	}
	for index < len(message) {
		value := message[index]
		if value == ' ' || value == '\t' || value == '\r' || value == '\n' {
			break
		}
		index++
	}
	return index
}

func doctorMarkerTokenByte04358(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' ||
		value >= 'a' && value <= 'z' || value == '_'
}
