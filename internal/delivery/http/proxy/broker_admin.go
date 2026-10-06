package proxy

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

const (
	brokerHealthPath          = "/__godex/runtime/health"
	brokerMetricsPath         = "/__godex/runtime/metrics"
	brokerActivatePath        = "/__godex/runtime/activate"
	brokerReleaseAffinityPath = "/__godex/runtime/session-affinity/release"
	brokerLogSnapshotPath     = "/__godex/runtime/log/snapshot"
	brokerLogEventPath        = "/__godex/runtime/log/event"
	brokerAdminTokenHeader    = "X-Godex-Admin-Token"
	brokerActivationMaxBytes  = 64 * 1024
	brokerLogEventMaxBytes    = 8 * 1024
)

type brokerHealth struct {
	PID               uint32  `json:"pid"`
	StartedAt         int64   `json:"started_at"`
	CurrentProfile    string  `json:"current_profile"`
	IncludeCodeReview bool    `json:"include_code_review"`
	ActiveRequests    int     `json:"active_requests"`
	InstanceID        string  `json:"instance_id"`
	PersistenceRole   string  `json:"persistence_role"`
	GodexVersion      *string `json:"godex_version,omitempty"`
	ExecutablePath    *string `json:"executable_path,omitempty"`
	ExecutableSHA256  *string `json:"executable_sha256,omitempty"`
}

type brokerWaitMetrics struct {
	WaitTotalNS uint64 `json:"wait_total_ns"`
	WaitCount   uint64 `json:"wait_count"`
	WaitMaxNS   uint64 `json:"wait_max_ns"`
}

type brokerLaneMetrics struct {
	Active                     int    `json:"active"`
	Limit                      int    `json:"limit"`
	AdmissionsTotal            uint64 `json:"admissions_total"`
	ReleasesTotal              uint64 `json:"releases_total"`
	GlobalLimitRejectionsTotal uint64 `json:"global_limit_rejections_total"`
	LaneLimitRejectionsTotal   uint64 `json:"lane_limit_rejections_total"`
	ReleaseUnderflowsTotal     uint64 `json:"release_underflows_total"`
}

type brokerTrafficMetrics struct {
	Responses brokerLaneMetrics `json:"responses"`
	Compact   brokerLaneMetrics `json:"compact"`
	WebSocket brokerLaneMetrics `json:"websocket"`
	Standard  brokerLaneMetrics `json:"standard"`
}

type brokerContinuationSignals struct {
	Response  int `json:"response"`
	TurnState int `json:"turn_state"`
	SessionID int `json:"session_id"`
}

type brokerContinuationMetrics struct {
	ResponseBindings      int                       `json:"response_bindings"`
	TurnStateBindings     int                       `json:"turn_state_bindings"`
	SessionIDBindings     int                       `json:"session_id_bindings"`
	Warm                  int                       `json:"warm"`
	Verified              int                       `json:"verified"`
	Suspect               int                       `json:"suspect"`
	Dead                  int                       `json:"dead"`
	FailureCounts         brokerContinuationSignals `json:"failure_counts"`
	NotFoundStreaks       brokerContinuationSignals `json:"not_found_streaks"`
	StaleVerifiedBindings brokerContinuationSignals `json:"stale_verified_bindings"`
}

type brokerRouteContinuityMetrics struct {
	Responses int `json:"responses"`
	Compact   int `json:"compact"`
	WebSocket int `json:"websocket"`
	Standard  int `json:"standard"`
}

type brokerPreviousContinuityMetrics struct {
	NegativeCacheEntries  brokerRouteContinuityMetrics `json:"negative_cache_entries"`
	NegativeCacheFailures brokerRouteContinuityMetrics `json:"negative_cache_failures"`
}

type brokerContinuityFailureReasons struct {
	ChainRetriedOwner          map[string]int `json:"chain_retried_owner"`
	ChainDeadUpstreamConfirmed map[string]int `json:"chain_dead_upstream_confirmed"`
	StaleContinuation          map[string]int `json:"stale_continuation"`
}

type brokerMetrics struct {
	Health                                brokerHealth                    `json:"health"`
	ActiveRequestLimit                    int                             `json:"active_request_limit"`
	LocalOverloadBackoffRemainingSeconds  uint64                          `json:"local_overload_backoff_remaining_seconds"`
	RuntimeStateLockWait                  brokerWaitMetrics               `json:"runtime_state_lock_wait"`
	AdmissionWait                         brokerWaitMetrics               `json:"admission_wait"`
	LongLivedQueueWait                    brokerWaitMetrics               `json:"long_lived_queue_wait"`
	Traffic                               brokerTrafficMetrics            `json:"traffic"`
	ProfileInflight                       map[string]int                  `json:"profile_inflight"`
	ActiveRequestReleaseUnderflowsTotal   uint64                          `json:"active_request_release_underflows_total"`
	ProfileInflightAdmissionsTotal        uint64                          `json:"profile_inflight_admissions_total"`
	ProfileInflightReleasesTotal          uint64                          `json:"profile_inflight_releases_total"`
	ProfileInflightReleaseUnderflowsTotal uint64                          `json:"profile_inflight_release_underflows_total"`
	RetryBackoffs                         int                             `json:"retry_backoffs"`
	TransportBackoffs                     int                             `json:"transport_backoffs"`
	RouteCircuits                         int                             `json:"route_circuits"`
	DegradedProfiles                      int                             `json:"degraded_profiles"`
	DegradedRoutes                        int                             `json:"degraded_routes"`
	Continuations                         brokerContinuationMetrics       `json:"continuations"`
	PreviousResponseContinuity            brokerPreviousContinuityMetrics `json:"previous_response_continuity"`
	ContinuityFailureReasons              brokerContinuityFailureReasons  `json:"continuity_failure_reasons"`
}

func (proxy *Proxy) handleBrokerAdmin(writer http.ResponseWriter, request *http.Request) bool {
	path := request.URL.Path
	if strings.HasPrefix(path, "/__prodex/runtime/") {
		path = "/__godex/runtime/" + strings.TrimPrefix(path, "/__prodex/runtime/")
	}
	switch path {
	case brokerHealthPath, brokerMetricsPath, brokerActivatePath, brokerReleaseAffinityPath,
		brokerLogSnapshotPath, brokerLogEventPath:
	default:
		return false
	}
	if proxy.broker == nil {
		writeBrokerError(writer, http.StatusNotFound, "not_found", "runtime broker admin endpoint is not enabled for this proxy")
		return true
	}
	token := strings.TrimSpace(request.Header.Get(brokerAdminTokenHeader))
	if token == "" {
		token = strings.TrimSpace(request.Header.Get("X-Prodex-Admin-Token"))
	}
	if !brokerTokenMatches(proxy.broker.AdminToken, token) {
		writeBrokerError(writer, http.StatusForbidden, "forbidden", "missing or invalid runtime broker admin token")
		return true
	}
	switch path {
	case brokerHealthPath:
		if request.Method != http.MethodGet {
			writeBrokerError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "runtime broker health requires GET")
			return true
		}
		writeBrokerJSON(writer, http.StatusOK, proxy.brokerHealth())
	case brokerMetricsPath:
		if request.Method != http.MethodGet {
			writeBrokerError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "runtime broker metrics requires GET")
			return true
		}
		writeBrokerJSON(writer, http.StatusOK, proxy.brokerMetrics())
	case brokerActivatePath:
		proxy.handleBrokerActivate(writer, request)
	case brokerReleaseAffinityPath:
		proxy.handleBrokerReleaseAffinity(writer, request)
	case brokerLogSnapshotPath:
		proxy.handleBrokerLogSnapshot(writer, request)
	case brokerLogEventPath:
		proxy.handleBrokerLogEvent(writer, request)
	}
	return true
}

func brokerTokenMatches(expected, candidate string) bool {
	if expected == "" || candidate == "" {
		return false
	}
	difference := len(expected) ^ len(candidate)
	for index := 0; index < len(expected); index++ {
		var other byte
		if index < len(candidate) {
			other = candidate[index]
		}
		difference |= int(expected[index] ^ other)
	}
	return subtle.ConstantTimeEq(int32(difference), 0) == 1
}

func (proxy *Proxy) brokerHealth() brokerHealth {
	proxy.mu.Lock()
	config := *proxy.broker
	proxy.mu.Unlock()
	role := strings.TrimSpace(config.PersistenceRole)
	if role == "" {
		role = "owner"
	}
	health := brokerHealth{
		PID: uint32(os.Getpid()), StartedAt: config.StartedAt,
		CurrentProfile: config.CurrentProfile, IncludeCodeReview: config.IncludeCodeReview,
		ActiveRequests: proxy.ActiveRequests(), InstanceID: config.InstanceID, PersistenceRole: role,
	}
	if config.GodexVersion != "" {
		health.GodexVersion = &config.GodexVersion
	}
	if config.ExecutablePath != "" {
		health.ExecutablePath = &config.ExecutablePath
	}
	if config.ExecutableSHA256 != "" {
		health.ExecutableSHA256 = &config.ExecutableSHA256
	}
	return health
}

func (proxy *Proxy) brokerMetrics() brokerMetrics {
	admission := proxy.admission.snapshot()
	lane := func(kind admissionLane) brokerLaneMetrics {
		current := admission.Lanes[kind]
		return brokerLaneMetrics{
			Active: current.Active, Limit: current.Limit,
			AdmissionsTotal: current.AdmissionsTotal, ReleasesTotal: current.ReleasesTotal,
			GlobalLimitRejectionsTotal: current.GlobalLimitRejectionsTotal,
			LaneLimitRejectionsTotal:   current.LaneLimitRejectionsTotal,
			ReleaseUnderflowsTotal:     current.ReleaseUnderflowsTotal,
		}
	}
	routing := proxy.router.BrokerMetricsSnapshot()
	return brokerMetrics{
		Health: proxy.brokerHealth(), ActiveRequestLimit: admission.GlobalLimit,
		AdmissionWait: brokerWaitMetrics{
			WaitTotalNS: admission.Wait.TotalNS,
			WaitCount:   admission.Wait.Count,
			WaitMaxNS:   admission.Wait.MaxNS,
		},
		Traffic: brokerTrafficMetrics{
			Responses: lane(admissionLaneResponses),
			Compact:   lane(admissionLaneCompact),
			WebSocket: lane(admissionLaneWebSocket),
			Standard:  lane(admissionLaneStandard),
		},
		ProfileInflight:                       routing.ProfileInflight,
		ActiveRequestReleaseUnderflowsTotal:   admission.ActiveRequestReleaseUnderflows,
		ProfileInflightAdmissionsTotal:        routing.ProfileInflightAdmissionsTotal,
		ProfileInflightReleasesTotal:          routing.ProfileInflightReleasesTotal,
		ProfileInflightReleaseUnderflowsTotal: routing.ProfileInflightReleaseUnderflowsTotal,
		RetryBackoffs:                         routing.RetryBackoffs,
		TransportBackoffs:                     routing.TransportBackoffs,
		RouteCircuits:                         routing.RouteCircuits,
		DegradedProfiles:                      routing.DegradedProfiles,
		DegradedRoutes:                        routing.DegradedRoutes,
		Continuations:                         brokerContinuationMetricsFromRouting(routing.Continuations),
		PreviousResponseContinuity:            brokerPreviousContinuityFromRouting(routing.PreviousResponseContinuity),
		ContinuityFailureReasons:              brokerContinuityReasons(proxy.brokerLog.snapshot(0, brokerLiveLogMaxEntries)),
	}
}

func brokerContinuationMetricsFromRouting(value routingusecase.BrokerContinuationMetrics) brokerContinuationMetrics {
	return brokerContinuationMetrics{
		ResponseBindings:  value.ResponseBindings,
		TurnStateBindings: value.TurnStateBindings,
		SessionIDBindings: value.SessionIDBindings,
		Warm:              value.Warm,
		Verified:          value.Verified,
		Suspect:           value.Suspect,
		Dead:              value.Dead,
		FailureCounts: brokerContinuationSignals{
			Response:  value.FailureCounts.Response,
			TurnState: value.FailureCounts.TurnState,
			SessionID: value.FailureCounts.SessionID,
		},
		NotFoundStreaks: brokerContinuationSignals{
			Response:  value.NotFoundStreaks.Response,
			TurnState: value.NotFoundStreaks.TurnState,
			SessionID: value.NotFoundStreaks.SessionID,
		},
		StaleVerifiedBindings: brokerContinuationSignals{
			Response:  value.StaleVerifiedBindings.Response,
			TurnState: value.StaleVerifiedBindings.TurnState,
			SessionID: value.StaleVerifiedBindings.SessionID,
		},
	}
}

func brokerPreviousContinuityFromRouting(value routingusecase.BrokerPreviousResponseContinuityMetrics) brokerPreviousContinuityMetrics {
	return brokerPreviousContinuityMetrics{
		NegativeCacheEntries: brokerRouteContinuityMetrics{
			Responses: value.NegativeCacheEntries.Responses,
			Compact:   value.NegativeCacheEntries.Compact,
			WebSocket: value.NegativeCacheEntries.WebSocket,
			Standard:  value.NegativeCacheEntries.Standard,
		},
		NegativeCacheFailures: brokerRouteContinuityMetrics{
			Responses: value.NegativeCacheFailures.Responses,
			Compact:   value.NegativeCacheFailures.Compact,
			WebSocket: value.NegativeCacheFailures.WebSocket,
			Standard:  value.NegativeCacheFailures.Standard,
		},
	}
}

func (proxy *Proxy) handleBrokerActivate(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeBrokerError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "runtime broker activation requires POST")
		return
	}
	body, tooLarge, err := readBrokerBody(request, brokerActivationMaxBytes)
	if tooLarge {
		writeBrokerError(writer, http.StatusRequestEntityTooLarge, "request_too_large",
			"runtime broker activation body exceeds 65536 bytes")
		return
	}
	if err != nil {
		writeBrokerError(writer, http.StatusBadRequest, "invalid_request", "runtime broker activation body is invalid")
		return
	}
	var payload struct {
		CurrentProfile string `json:"current_profile"`
	}
	if json.Unmarshal(body, &payload) != nil || strings.TrimSpace(payload.CurrentProfile) == "" {
		writeBrokerError(writer, http.StatusBadRequest, "invalid_request",
			"runtime broker activation requires a non-empty current_profile")
		return
	}
	profile := strings.TrimSpace(payload.CurrentProfile)
	if proxy.broker.ResolveProfile == nil {
		writeBrokerError(writer, http.StatusInternalServerError, "internal_error", "runtime broker profile resolver is unavailable")
		return
	}
	accountID, err := proxy.broker.ResolveProfile(request.Context(), profile)
	if err != nil || strings.TrimSpace(accountID) == "" {
		message := "runtime broker activation profile is unavailable"
		if err != nil {
			message = err.Error()
		}
		writeBrokerError(writer, http.StatusInternalServerError, "internal_error", message)
		return
	}
	proxy.router.SetPreferredAccount(accountID)
	proxy.mu.Lock()
	proxy.broker.CurrentProfile = profile
	onActivated := proxy.broker.OnActivated
	proxy.mu.Unlock()
	if proxy.brokerLog != nil {
		proxy.brokerLog.append("runtime_broker_activate current_profile=" + profile)
	}
	if onActivated != nil {
		if err := onActivated(request.Context(), profile); err != nil {
			writeBrokerError(writer, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	writeBrokerJSON(writer, http.StatusOK, map[string]any{"ok": true, "current_profile": profile})
}

func (proxy *Proxy) handleBrokerReleaseAffinity(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeBrokerError(writer, http.StatusMethodNotAllowed, "method_not_allowed",
			"runtime broker session affinity release requires POST")
		return
	}
	body, tooLarge, err := readBrokerBody(request, brokerActivationMaxBytes)
	if tooLarge {
		writeBrokerError(writer, http.StatusRequestEntityTooLarge, "request_too_large",
			"runtime broker activation body exceeds 65536 bytes")
		return
	}
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err != nil || json.Unmarshal(body, &payload) != nil {
		writeBrokerError(writer, http.StatusBadRequest, "invalid_request",
			"runtime broker session affinity release requires a valid session_id")
		return
	}
	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" || len(sessionID) > 256 {
		writeBrokerError(writer, http.StatusBadRequest, "invalid_request",
			"runtime broker session affinity release requires a valid session_id")
		return
	}
	if err := proxy.router.ReleaseSessionAffinity(request.Context(), sessionID); err != nil {
		writeBrokerError(writer, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeBrokerJSON(writer, http.StatusOK, map[string]any{"ok": true})
}

func (proxy *Proxy) handleBrokerLogSnapshot(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeBrokerError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "runtime broker log snapshot requires GET")
		return
	}
	after := uint64(0)
	if value := request.URL.Query().Get("after"); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			writeBrokerError(writer, http.StatusBadRequest, "invalid_request", "runtime log snapshot cursor is invalid")
			return
		}
		after = parsed
	}
	limit := 512
	if value := request.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeBrokerError(writer, http.StatusBadRequest, "invalid_request", "runtime log snapshot limit is invalid")
			return
		}
		if parsed < 1 || parsed > 512 {
			writeBrokerError(writer, http.StatusBadRequest, "invalid_request",
				"runtime log snapshot limit is outside the supported bound")
			return
		}
		limit = parsed
	}
	writeBrokerJSON(writer, http.StatusOK, proxy.brokerLog.snapshot(after, limit))
}

func (proxy *Proxy) handleBrokerLogEvent(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeBrokerError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "runtime broker log event requires POST")
		return
	}
	body, tooLarge, err := readBrokerBody(request, brokerLogEventMaxBytes)
	if err != nil || tooLarge {
		writeBrokerError(writer, http.StatusBadRequest, "invalid_request", "runtime broker log event is invalid")
		return
	}
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Message == "" || len(payload.Message) > 4*1024 ||
		!strings.Contains(payload.Message, "runtime_recovery") {
		writeBrokerError(writer, http.StatusBadRequest, "invalid_request", "runtime broker log event is invalid")
		return
	}
	if proxy.broker.LogRecovery != nil {
		if err := proxy.broker.LogRecovery(request.Context(), payload.Message); err != nil {
			writeBrokerError(writer, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	if proxy.brokerLog != nil {
		proxy.brokerLog.append(payload.Message)
	}
	writeBrokerJSON(writer, http.StatusOK, map[string]any{"ok": true})
}

func readBrokerBody(request *http.Request, limit int64) ([]byte, bool, error) {
	if request.Body == nil {
		return nil, false, nil
	}
	defer request.Body.Close()
	body, err := io.ReadAll(io.LimitReader(request.Body, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return nil, true, nil
	}
	return body, false, nil
}

func writeBrokerError(writer http.ResponseWriter, status int, code, message string) {
	writeBrokerJSON(writer, status, map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}

func writeBrokerJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
