package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Prodex 0.436.1 has process-lifetime state-save, continuation-journal, and
// probe-refresh workers. Godex intentionally has no corresponding async
// queues; an unset PressureSnapshot must not invent backlog pressure.
func TestProdex04361AbsentBackgroundQueuesDoNotShedFreshCompact(t *testing.T) {
	handler, gateway := admissionOwner04360Setup(t)
	if handler.pressureSnapshot != nil {
		t.Fatal("background queue pressure provider unexpectedly configured")
	}
	if handler.pressureMode(admissionLaneCompact) {
		t.Fatal("absent state-save, continuation-journal, or probe-refresh queues reported pressure")
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/backend-api/codex/responses/compact",
		strings.NewReader(`{"model":"test","input":[]}`),
	))
	if response.Code != http.StatusOK || gateway.calls.Load() != 1 {
		t.Fatalf("fresh compact was shed without a queue provider: status=%d calls=%d", response.Code, gateway.calls.Load())
	}
}
