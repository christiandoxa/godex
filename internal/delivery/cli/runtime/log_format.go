package runtime

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/redact"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	"golang.org/x/term"
)

const defaultLogRenderWidth = 100

var exactLogEventNames = map[string]string{
	"request_captured":                           "received request",
	"route_decision":                             "route decided",
	"selection_plan":                             "route planned",
	"selection_pick":                             "profile picked",
	"selection_keep_affinity":                    "owner kept",
	"selection_keep_current":                     "current profile kept",
	"selection_skip_current":                     "profile skipped",
	"selection_skip_affinity":                    "affinity skipped",
	"selection_skip_sync_probe":                  "quota probe skipped",
	"local_selection_blocked":                    "route blocked",
	"profile_commit":                             "profile committed",
	"route_affinity_recompute":                   "affinity recomputed",
	"route_affinity_recompute_result":            "affinity resolved",
	"previous_response_owner":                    "continuation owner",
	"previous_response_not_found":                "continuation missing",
	"previous_response_negative_cache":           "continuation cached missing",
	"previous_response_fresh_fallback":           "fresh fallback",
	"previous_response_fresh_fallback_blocked":   "fallback blocked",
	"previous_response_turn_state_rehydrated":    "turn state restored",
	"session_rotation_release_affinity":          "session affinity released",
	"binding_prompt_cache":                       "prompt cache bound",
	"upgrade":                                    "request upgraded",
	"upgraded":                                   "request upgraded",
	"profile_quota_exhausted":                    "quota exhausted",
	"quota_exhausted":                            "quota exhausted",
	"quota_blocked":                              "quota blocked",
	"quota_critical_floor_before_send":           "quota floor blocked",
	"profile_quota_quarantine":                   "quota quarantine",
	"profile_probe_refresh_start":                "quota refresh started",
	"profile_probe_refresh_ok":                   "quota refreshed",
	"upstream_usage_limit_passthrough":           "upstream limit passed through",
	"upstream_overload_passthrough":              "upstream overload passed through",
	"profile_retry_backoff":                      "retry backoff",
	"compact_retryable_failure":                  "compaction retry",
	"compact_overload_conservative_retry":        "compaction retry (overload)",
	"profile_transport_backoff":                  "transport backoff",
	"rotation_waiting_for_recovery":              "waiting for recovery",
	"profile_circuit_open":                       "circuit open",
	"profile_circuit_half_open_probe":            "circuit probe",
	"profile_transport_failure":                  "transport failed",
	"profile_health":                             "health penalty",
	"profile_bad_pairing":                        "affinity penalty",
	"upstream_start":                             "upstream request",
	"upstream_async_start":                       "upstream request",
	"upstream_response":                          "upstream response",
	"upstream_async_response":                    "upstream response",
	"upstream_connect_start":                     "upstream connecting",
	"upstream_connect_ok":                        "upstream connected",
	"upstream_connect_error":                     "upstream connect failed",
	"first_upstream_chunk":                       "first upstream chunk",
	"first_local_chunk":                          "first local chunk",
	"stream_complete":                            "stream complete",
	"buffered_response_complete":                 "response complete",
	"terminal_event":                             "terminal event",
	"runtime_proxy_queue_overloaded":             "proxy queue full",
	"runtime_proxy_active_limit_reached":         "proxy busy",
	"runtime_proxy_lane_limit_reached":           "lane full",
	"profile_inflight_saturated":                 "profile busy",
	"smart_context_autopilot":                    "Smart Context",
	"smart_context_prepare_error":                "Smart Context failed",
	"smart_context_prepare_fallback":             "Smart Context fallback",
	"smart_context_disabled":                     "Smart Context disabled",
	"local_rewrite_request_detail":               "provider request",
	"local_rewrite_provider_model_fallback":      "model fallback",
	"local_rewrite_provider_auth_failure":        "provider auth failed",
	"upstream_read_error":                        "upstream read failed",
	"upstream_send_error":                        "upstream send failed",
	"upstream_stream_error":                      "upstream stream failed",
	"upstream_close_before_completed":            "upstream closed early",
	"upstream_connection_closed":                 "upstream disconnected",
	"stream_read_error":                          "stream read failed",
	"local_writer_error":                         "terminal write failed",
	"invalid_previous_response_id":               "continuation invalid",
	"session_error":                              "session failed",
	"local_connection_closed":                    "local connection closed",
	"profile_probe_refresh_error":                "quota refresh failed",
	"smart_context_token_calibration_save_error": "Smart Context calibration failed",
}

func humanLogEventName(event string) string {
	event = strings.TrimSpace(event)
	if label, ok := exactLogEventNames[event]; ok {
		return label
	}
	lower := strings.ToLower(event)
	switch {
	case strings.Contains(lower, "compact"), strings.Contains(lower, "compaction"):
		return "compaction"
	case strings.Contains(lower, "mcp"), strings.HasPrefix(lower, "expose_"), strings.HasPrefix(lower, "super_expose_"):
		return "MCP"
	case strings.Contains(lower, "sub_agent"), strings.Contains(lower, "subagent"):
		return "sub-agent"
	default:
		return strings.ReplaceAll(event, "_", " ")
	}
}

func localLogTimestamp(unixMilli int64) string {
	return time.UnixMilli(unixMilli).In(time.Local).Format("2006-01-02 15:04:05.000 -07:00")
}

func logEventSource(kind string) string {
	switch {
	case kind == "request_started", kind == "request_completed", kind == "request_failed":
		return "REQUEST"
	case kind == "profile_health", kind == "profile_latency":
		return "HEALTH"
	case kind == "profile_inflight_saturated",
		kind == "runtime_proxy_queue_overloaded",
		kind == "runtime_proxy_active_limit_reached",
		kind == "runtime_proxy_lane_limit_reached":
		return "LOAD"
	case strings.Contains(kind, "compact"), strings.Contains(kind, "compaction"):
		return "COMPACT"
	case strings.Contains(kind, "mcp"), strings.HasPrefix(kind, "expose_"), strings.HasPrefix(kind, "super_expose_"):
		return "MCP"
	case strings.Contains(kind, "sub_agent"), strings.Contains(kind, "subagent"):
		return "AGENT"
	default:
		return "EVENT"
	}
}

func logEventMeta(event runtimemodel.Event) [][2]string {
	meta := make([][2]string, 0, len(event.Fields)+7)
	appendIf := func(key, value string) {
		if strings.TrimSpace(value) != "" && value != "-" {
			meta = append(meta, [2]string{key, redact.Secrets(value)})
		}
	}
	appendIf("request", event.RequestID)
	appendIf("profile", event.AccountID)
	if event.StatusCode != 0 {
		appendIf("status", fmt.Sprint(event.StatusCode))
	}
	appendIf("method", event.Method)
	appendIf("path", event.Path)
	if event.DurationMillis != 0 {
		appendIf("duration_ms", fmt.Sprint(event.DurationMillis))
	}
	keys := make([]string, 0, len(event.Fields))
	for key := range event.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seen := make(map[string]bool, len(meta))
	for _, pair := range meta {
		seen[pair[0]] = true
	}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		appendIf(key, event.Fields[key])
	}
	return meta
}

func formatLogEventWidth(event runtimemodel.Event, width int) string {
	if width < 24 {
		width = 24
	}
	body := []string{humanLogEventName(event.Kind)}
	if message := strings.TrimSpace(redact.Secrets(event.Message)); message != "" {
		body = append(body, message)
	}
	return strings.Join(renderLogBlock(localLogTimestamp(event.TimestampUnixMilli), logEventSource(event.Kind), logEventMeta(event), body, width), "\n")
}

func renderLogBlock(timestamp, title string, meta [][2]string, body []string, width int) []string {
	prefix := "[" + timestamp + "] " + title
	lines := []string{prefix}
	if len(prefix)+1 < width {
		lines[0] = prefix + " " + strings.Repeat("-", width-len(prefix)-1)
	}
	if len(meta) > 0 {
		parts := make([]string, 0, len(meta))
		for _, pair := range meta {
			parts = append(parts, pair[0]+"="+pair[1])
		}
		lines = append(lines, wrapLogLine(strings.Join(parts, "  "), max(width-2, 20), "  ")...)
	}
	for _, block := range body {
		lines = append(lines, wrapLogLine(block, max(width-4, 20), "  | ")...)
	}
	return lines
}

func wrapLogLine(text string, width int, prefix string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	maxContent := max(width-len(prefix), 1)
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{prefix}
	}
	lines := make([]string, 0, 1)
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) <= maxContent {
			current += " " + word
			continue
		}
		lines = append(lines, prefix+current)
		current = word
	}
	lines = append(lines, prefix+current)
	return lines
}

func logWriterWidth(out io.Writer) int {
	file, ok := out.(*os.File)
	if !ok {
		return defaultLogRenderWidth
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width < 24 {
		return defaultLogRenderWidth
	}
	return width
}
