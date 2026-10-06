package routing

import "time"

const (
	continuationSuspectGrace   = 120 * time.Second
	continuationDeadGrace      = 15 * time.Minute
	continuationVerifiedStale  = 30 * time.Minute
	continuationSuspectLimit   = uint32(2)
	continuationConfidenceMax  = uint32(8)
	continuationVerifiedBonus  = uint32(2)
	continuationTouchBonus     = uint32(1)
	continuationSuspectPenalty = uint32(1)
)

type continuationLifecycle uint8

const (
	continuationWarm continuationLifecycle = iota
	continuationVerified
	continuationSuspect
	continuationDead
)

type continuationStatus struct {
	kind           string
	state          continuationLifecycle
	confidence     uint32
	lastTouchedAt  int64
	lastVerifiedAt int64
	lastNotFoundAt int64
	notFoundStreak uint32
	successCount   uint32
	failureCount   uint32
}

func continuationStatusKind(bindingKind string) string {
	switch bindingKind {
	case "previous":
		return "response"
	case "turn":
		return "turn_state"
	case "session":
		return "session_id"
	default:
		return ""
	}
}

func continuationNextEvent(status continuationStatus, now int64) int64 {
	last := max(status.lastTouchedAt, status.lastVerifiedAt, status.lastNotFoundAt)
	if last >= now {
		return last + 1
	}
	return now
}

func continuationTouch(status continuationStatus, now int64) continuationStatus {
	eventAt := continuationNextEvent(status, now)
	status.lastTouchedAt = eventAt
	if status.state == continuationSuspect {
		if status.lastNotFoundAt > 0 && eventAt-status.lastNotFoundAt >= int64(continuationSuspectGrace/time.Second) {
			status.state = continuationWarm
			status.lastNotFoundAt = 0
			status.notFoundStreak = 0
		}
		status.confidence = min(status.confidence+continuationTouchBonus, continuationConfidenceMax)
	} else if status.state != continuationDead {
		status.confidence = min(status.confidence+continuationTouchBonus, continuationConfidenceMax)
	}
	return status
}

func continuationVerify(status continuationStatus, now int64) continuationStatus {
	eventAt := continuationNextEvent(status, now)
	status.state = continuationVerified
	status.confidence = min(status.confidence+continuationVerifiedBonus, continuationConfidenceMax)
	status.lastTouchedAt = eventAt
	status.lastVerifiedAt = eventAt
	status.lastNotFoundAt = 0
	status.notFoundStreak = 0
	status.successCount++
	status.failureCount = 0
	return status
}

func continuationMarkSuspect(status continuationStatus, now int64) continuationStatus {
	eventAt := continuationNextEvent(status, now)
	previousConfidence := status.confidence
	if status.confidence >= continuationSuspectPenalty {
		status.confidence -= continuationSuspectPenalty
	} else {
		status.confidence = 0
	}
	if previousConfidence == 0 {
		status.confidence = 1
	}
	status.notFoundStreak++
	status.failureCount++
	if status.notFoundStreak >= continuationSuspectLimit || (previousConfidence > 0 && status.confidence == 0) {
		status.state = continuationDead
	} else {
		status.state = continuationSuspect
	}
	status.lastTouchedAt = eventAt
	status.lastNotFoundAt = eventAt
	return status
}

func continuationMarkDead(status continuationStatus, now int64) continuationStatus {
	eventAt := continuationNextEvent(status, now)
	status.state = continuationDead
	status.confidence = 0
	status.lastTouchedAt = eventAt
	status.lastNotFoundAt = eventAt
	status.notFoundStreak = max(status.notFoundStreak, continuationSuspectLimit)
	status.failureCount++
	return status
}

func (store *affinityStore) touchContinuationEntriesLocked(entries []bindingEntry, now time.Time, verified bool) {
	if store.statuses == nil {
		store.statuses = make(map[string]continuationStatus)
	}
	for _, entry := range entries {
		kind := continuationStatusKind(entry.Kind)
		if kind == "" {
			continue
		}
		status := store.statuses[entry.Key]
		status.kind = kind
		if verified {
			status = continuationVerify(status, now.Unix())
		} else {
			status = continuationTouch(status, now.Unix())
		}
		store.statuses[entry.Key] = status
	}
	store.pruneContinuationStatusesLocked(now)
}

type bindingEntry struct {
	Kind string
	Key  string
}

func continuationEntries(keys affinityKeys) []bindingEntry {
	bindings := keys.entries()
	result := make([]bindingEntry, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, bindingEntry{Kind: binding.Kind, Key: binding.Key})
	}
	return result
}

func (store *affinityStore) markContinuationSuspect(kind, key string, now time.Time) {
	if kind == "" || key == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.statuses == nil {
		store.statuses = make(map[string]continuationStatus)
	}
	status := store.statuses[key]
	status.kind = kind
	store.statuses[key] = continuationMarkSuspect(status, now.Unix())
	store.pruneContinuationStatusesLocked(now)
}

func (store *affinityStore) markContinuationDeadLocked(kind, key string, now time.Time) {
	if kind == "" || key == "" {
		return
	}
	if store.statuses == nil {
		store.statuses = make(map[string]continuationStatus)
	}
	status := store.statuses[key]
	status.kind = kind
	store.statuses[key] = continuationMarkDead(status, now.Unix())
}

func (store *affinityStore) removeContinuationStatusLocked(key string) {
	if store.statuses != nil {
		delete(store.statuses, key)
	}
}

func (store *affinityStore) pruneContinuationStatusesLocked(now time.Time) {
	if len(store.statuses) == 0 {
		return
	}
	nowUnix := now.Unix()
	for key, status := range store.statuses {
		_, live := store.values[key]
		if live {
			continue
		}
		last := max(status.lastTouchedAt, status.lastVerifiedAt, status.lastNotFoundAt)
		switch status.state {
		case continuationDead:
			if last == 0 || nowUnix-last >= int64(continuationDeadGrace/time.Second) {
				delete(store.statuses, key)
			}
		case continuationSuspect:
			if status.lastNotFoundAt == 0 ||
				nowUnix-status.lastNotFoundAt >= int64(continuationSuspectGrace/time.Second) ||
				status.notFoundStreak >= continuationSuspectLimit ||
				status.confidence == 0 {
				delete(store.statuses, key)
			}
		}
	}
	pruneContinuationStatusKind(store.statuses, "response", 16_384)
	pruneContinuationStatusKind(store.statuses, "turn_state", 2_048)
	pruneContinuationStatusKind(store.statuses, "session_id", 2_048)
}

func pruneContinuationStatusKind(values map[string]continuationStatus, kind string, limit int) {
	count := 0
	for _, status := range values {
		if status.kind == kind {
			count++
		}
	}
	for count > limit {
		oldestKey := ""
		var oldest int64
		for key, status := range values {
			if status.kind != kind {
				continue
			}
			last := max(status.lastTouchedAt, status.lastVerifiedAt, status.lastNotFoundAt)
			if oldestKey == "" || last < oldest || (last == oldest && key < oldestKey) {
				oldestKey, oldest = key, last
			}
		}
		if oldestKey == "" {
			return
		}
		delete(values, oldestKey)
		count--
	}
}

type BrokerContinuationSignalMetrics struct {
	Response  int
	TurnState int
	SessionID int
}

type BrokerContinuationMetrics struct {
	ResponseBindings      int
	TurnStateBindings     int
	SessionIDBindings     int
	Warm                  int
	Verified              int
	Suspect               int
	Dead                  int
	FailureCounts         BrokerContinuationSignalMetrics
	NotFoundStreaks       BrokerContinuationSignalMetrics
	StaleVerifiedBindings BrokerContinuationSignalMetrics
}

type BrokerRouteContinuityMetrics struct {
	Responses int
	Compact   int
	WebSocket int
	Standard  int
}

type BrokerPreviousResponseContinuityMetrics struct {
	NegativeCacheEntries  BrokerRouteContinuityMetrics
	NegativeCacheFailures BrokerRouteContinuityMetrics
}

type BrokerMetricsSnapshot struct {
	ProfileInflight                       map[string]int
	ProfileInflightAdmissionsTotal        uint64
	ProfileInflightReleasesTotal          uint64
	ProfileInflightReleaseUnderflowsTotal uint64
	RetryBackoffs                         int
	TransportBackoffs                     int
	RouteCircuits                         int
	DegradedProfiles                      int
	DegradedRoutes                        int
	Continuations                         BrokerContinuationMetrics
	PreviousResponseContinuity            BrokerPreviousResponseContinuityMetrics
}

func (store *affinityStore) brokerContinuationMetrics(now time.Time) BrokerContinuationMetrics {
	if store == nil {
		return BrokerContinuationMetrics{}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneContinuationStatusesLocked(now)
	var result BrokerContinuationMetrics
	for _, status := range store.statuses {
		switch status.kind {
		case "response":
			result.ResponseBindings++
		case "turn_state":
			result.TurnStateBindings++
		case "session_id":
			result.SessionIDBindings++
		default:
			continue
		}
		switch status.state {
		case continuationWarm:
			result.Warm++
		case continuationVerified:
			result.Verified++
		case continuationSuspect:
			result.Suspect++
		case continuationDead:
			result.Dead++
		}
		addContinuationSignal(&result.FailureCounts, status.kind, int(status.failureCount))
		addContinuationSignal(&result.NotFoundStreaks, status.kind, int(status.notFoundStreak))
		last := max(status.lastTouchedAt, status.lastVerifiedAt, status.lastNotFoundAt)
		if status.state == continuationVerified && last > 0 &&
			now.Unix()-last >= int64(continuationVerifiedStale/time.Second) {
			addContinuationSignal(&result.StaleVerifiedBindings, status.kind, 1)
		}
	}
	return result
}

func addContinuationSignal(metrics *BrokerContinuationSignalMetrics, kind string, value int) {
	switch kind {
	case "response":
		metrics.Response += value
	case "turn_state":
		metrics.TurnState += value
	case "session_id":
		metrics.SessionID += value
	}
}

func addRouteContinuity(metrics *BrokerRouteContinuityMetrics, route string, value int) bool {
	switch route {
	case "responses":
		metrics.Responses += value
	case "compact":
		metrics.Compact += value
	case "websocket":
		metrics.WebSocket += value
	case "standard":
		metrics.Standard += value
	default:
		return false
	}
	return true
}

func (router *Router) BrokerMetricsSnapshot() BrokerMetricsSnapshot {
	if router == nil {
		return BrokerMetricsSnapshot{}
	}
	now := router.now()
	result := BrokerMetricsSnapshot{ProfileInflight: make(map[string]int)}

	router.mu.Lock()
	for accountID, count := range router.inflight {
		if count > 0 {
			result.ProfileInflight[accountID] = count
		}
	}
	result.ProfileInflightAdmissionsTotal = router.profileInflightAdmissionsTotal
	result.ProfileInflightReleasesTotal = router.profileInflightReleasesTotal
	result.ProfileInflightReleaseUnderflowsTotal = router.profileInflightReleaseUnderflowsTotal
	for _, state := range router.quarantine {
		if !state.authFailure && state.until.After(now) {
			result.RetryBackoffs++
		}
	}
	for key, score := range router.routeHealth {
		if score.Effective(now) == 0 {
			continue
		}
		if key.route == "global" {
			result.DegradedProfiles++
		} else {
			result.DegradedRoutes++
		}
	}
	router.mu.Unlock()

	router.transportMu.Lock()
	for _, backoff := range router.transportBackoffs {
		if backoff.Remaining(now) > 0 {
			result.TransportBackoffs++
		}
	}
	router.transportMu.Unlock()

	router.routeCircuitMu.Lock()
	for _, circuit := range router.routeCircuits {
		if circuit.Remaining(now) > 0 {
			result.RouteCircuits++
		}
	}
	router.routeCircuitMu.Unlock()

	router.previousResponseFailureMu.Lock()
	for _, failure := range router.previousResponseFailures {
		score := int(failure.Effective(now))
		if score == 0 {
			continue
		}
		if addRouteContinuity(&result.PreviousResponseContinuity.NegativeCacheEntries, failure.Route, 1) {
			addRouteContinuity(&result.PreviousResponseContinuity.NegativeCacheFailures, failure.Route, score)
		}
	}
	router.previousResponseFailureMu.Unlock()

	result.Continuations = router.affinity.brokerContinuationMetrics(now)
	return result
}
