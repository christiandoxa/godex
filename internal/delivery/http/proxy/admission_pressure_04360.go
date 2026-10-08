package proxy

import (
	"encoding/json"
	"net/http"
	"time"
)

// AdmissionPressure distinguishes actual local rejection/backoff from
// unrelated lane occupancy. Prodex 0.436.0 treats these three independent
// background queues as pressure sources. Each depth is the pending backlog,
// not the number of running workers.
type AdmissionPressure struct {
	LocalOverload              bool
	StateSaveBacklog           int
	ContinuationJournalBacklog int
	ProbeRefreshBacklog        int
}

const localOverloadBackoff = 3 * time.Second

func admissionPressureForLane(lane admissionLane, snapshot AdmissionPressure) bool {
	if snapshot.LocalOverload {
		return true
	}
	if lane != admissionLaneCompact && lane != admissionLaneStandard {
		return false
	}
	return snapshot.StateSaveBacklog >= 8 ||
		snapshot.ContinuationJournalBacklog >= 8 ||
		snapshot.ProbeRefreshBacklog >= 16
}

func (handler *activeRequestHandler) pressureMode(lane admissionLane) bool {
	if handler == nil {
		return false
	}
	handler.mu.Lock()
	now := handler.now
	if now == nil {
		now = time.Now
	}
	local := now().Before(handler.localOverloadUntil)
	source := handler.pressureSnapshot
	handler.mu.Unlock()
	signals := AdmissionPressure{LocalOverload: local}
	if source != nil {
		next := source()
		signals.LocalOverload = signals.LocalOverload || next.LocalOverload
		signals.StateSaveBacklog = next.StateSaveBacklog
		signals.ContinuationJournalBacklog = next.ContinuationJournalBacklog
		signals.ProbeRefreshBacklog = next.ProbeRefreshBacklog
	}
	return admissionPressureForLane(lane, signals)
}

func (handler *activeRequestHandler) markLocalOverload() {
	if handler == nil {
		return
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	now := handler.now
	if now == nil {
		now = time.Now
	}
	next := now().Add(localOverloadBackoff)
	if next.After(handler.localOverloadUntil) {
		handler.localOverloadUntil = next
	}
}

const freshCompactPressureMessage = "Fresh compact requests are temporarily deferred while the runtime proxy is under pressure. Retry the request."

func writeFreshCompactPressureResponse(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusServiceUnavailable)
	payload, _ := json.Marshal(struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{
		Code: "service_unavailable", Message: freshCompactPressureMessage,
	}})
	_, _ = writer.Write(payload)
}

// Local admission rejection is a separate event from ordinary saturation.
// Match Prodex 0.436.0's 3-second production Retry-After hint.
func writeLocalAdmissionOverloadResponse(writer http.ResponseWriter, lane admissionLane, websocket bool) {
	const message = "Runtime auto-rotate proxy is temporarily saturated. Retry the request."
	writer.Header().Set("Retry-After", "3")
	if !websocket && (lane == admissionLaneResponses || lane == admissionLaneCompact) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.WriteHeader(http.StatusServiceUnavailable)
		payload, _ := json.Marshal(map[string]any{"error": map[string]string{
			"code": "service_unavailable", "message": message,
		}})
		_, _ = writer.Write(payload)
		return
	}
	http.Error(writer, message, http.StatusServiceUnavailable)
}
