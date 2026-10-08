package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	routingusecase "github.com/christiandoxa/godex/internal/usecase/routing"
)

func TestProdex04360AdmissionPressurePolicyMatchesTaggedMojo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lane     admissionLane
		pressure AdmissionPressure
		want     bool
	}{
		{"local applies to responses", admissionLaneResponses, AdmissionPressure{LocalOverload: true}, true},
		{"local applies to compact", admissionLaneCompact, AdmissionPressure{LocalOverload: true}, true},
		{"background affects compact", admissionLaneCompact, AdmissionPressure{StateSaveBacklog: 8}, true},
		{"background affects standard", admissionLaneStandard, AdmissionPressure{ContinuationJournalBacklog: 8}, true},
		{"probe threshold compact", admissionLaneCompact, AdmissionPressure{ProbeRefreshBacklog: 16}, true},
		{"background does not affect responses", admissionLaneResponses, AdmissionPressure{StateSaveBacklog: 8}, false},
		{"background does not affect websocket", admissionLaneWebSocket, AdmissionPressure{ProbeRefreshBacklog: 16}, false},
		{"below thresholds", admissionLaneCompact, AdmissionPressure{StateSaveBacklog: 7, ContinuationJournalBacklog: 7, ProbeRefreshBacklog: 15}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := admissionPressureForLane(tc.lane, tc.pressure)
			if got != tc.want {
				t.Fatalf("pressure for %s=%t want %t", tc.name, got, tc.want)
			}
		})
	}
}

func TestProdex04360FreshCompactPressureShedProtectsOwnedContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		pressure   AdmissionPressure
		wantCode   int
	}{
		{"fresh local overload", `{"model":"test","input":[]}`, AdmissionPressure{LocalOverload: true}, 503},
		{"fresh backlog", `{"model":"test","input":[]}`, AdmissionPressure{StateSaveBacklog: 8}, 503},
		{"fresh under threshold", `{"model":"test","input":[]}`, AdmissionPressure{StateSaveBacklog: 7, ContinuationJournalBacklog: 7, ProbeRefreshBacklog: 15}, 200},
		{"bound session local", `{"session_id":"owned-session","input":[]}`, AdmissionPressure{LocalOverload: true}, 200},
		{"bound previous background", `{"previous_response_id":"owned-response","input":[]}`, AdmissionPressure{ContinuationJournalBacklog: 8}, 200},
		{"unbound invented session", `{"session_id":"invented","input":[]}`, AdmissionPressure{LocalOverload: true}, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &admissionOwner04360Gateway{}
			router, err := routingusecase.NewRouter(routingusecase.Config{
				Gateway: gateway,
				Accounts: func(context.Context) ([]proxymodel.Account, error) {
					return []proxymodel.Account{{ID: "a", Enabled: true, Home: "/a"}}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			if err = router.Observe(t.Context(), "a", nil, []byte(`{"response":{"id":"owned-response"},"session_id":"owned-session"}`), false); err != nil {
				t.Fatal(err)
			}
			proxy, err := NewProxy(Config{Router: router, PressureSnapshot: func() AdmissionPressure { return tc.pressure }})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/backend-api/codex/responses/compact", strings.NewReader(tc.body))
			recorder := httptest.NewRecorder()
			proxy.server.Handler.ServeHTTP(recorder, request)
			if recorder.Code != tc.wantCode {
				t.Fatalf("HTTP status=%d want %d body=%s", recorder.Code, tc.wantCode, recorder.Body.String())
			}
			if tc.wantCode == 503 {
				if gateway.calls.Load() != 0 {
					t.Fatal("shed compact dispatched to upstream")
				}
				var payload struct {
					Error struct{ Code, Message string }
				}
				if json.Unmarshal(recorder.Body.Bytes(), &payload) != nil ||
					payload.Error.Code != "service_unavailable" ||
					!strings.Contains(payload.Error.Message, "Fresh compact requests are temporarily deferred") {
					t.Fatalf("Prodex compact pressure payload changed: %s", recorder.Body.String())
				}
			} else if gateway.calls.Load() != 1 {
				t.Fatalf("owned/ordinary compact was not dispatched: %d", gateway.calls.Load())
			}
		})
	}
}

func TestProdex04360LocalOverloadExpiresAndNeverDerivesFromLaneSaturation(t *testing.T) {
	handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), admissionLimits{
		global: 2, lane: [admissionLaneCount]int{1, 1, 1, 1},
	}).(*activeRequestHandler)
	now := time.Unix(100, 0)
	handler.now = func() time.Time { return now }
	handler.active = 1
	handler.laneActive[admissionLaneCompact] = 1
	if handler.pressureMode(admissionLaneCompact) {
		t.Fatal("mere admission saturation incorrectly marks local overload")
	}
	handler.markLocalOverload()
	if !handler.pressureMode(admissionLaneCompact) {
		t.Fatal("real local admission rejection did not activate pressure")
	}
	now = now.Add(3 * time.Second)
	if handler.pressureMode(admissionLaneCompact) {
		t.Fatal("local backoff did not expire")
	}
}

// Only an actual admission refusal sets the short local-overload window.
// Ordinary capacity waits must not convert into overload pressure.
func TestProdex04360AdmissionRejectionMarksOverloadAndRetryAfter(t *testing.T) {
	handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("rejected request dispatched")
	}), admissionLimits{global: 1, lane: [admissionLaneCount]int{1, 1, 1, 1}}).(*activeRequestHandler)
	handler.active = 1
	handler.laneActive[admissionLaneCompact] = 1
	handler.wait = func(context.Context, <-chan struct{}) bool { return false }
	if handler.pressureMode(admissionLaneCompact) {
		t.Fatal("mere saturation activated pressure")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/backend-api/codex/responses/compact", strings.NewReader("{}")))
	if response.Code != 503 || response.Header().Get("Retry-After") != "3" {
		t.Fatalf("local refusal status/retry=%d/%q body=%s", response.Code, response.Header().Get("Retry-After"), response.Body.String())
	}
	if !handler.pressureMode(admissionLaneCompact) {
		t.Fatal("rejection did not mark local backoff")
	}
}

func TestProdex04360BackgroundQueuePressureShedsOptionalMetadataBeforeHalfLane(t *testing.T) {
	for _, snapshot := range []AdmissionPressure{
		{LocalOverload: true},
		{StateSaveBacklog: 8},
		{ContinuationJournalBacklog: 8},
		{ProbeRefreshBacklog: 16},
	} {
		proxy, err := NewProxy(Config{
			Router:           pressureTestRouter04360(t),
			PressureSnapshot: func() AdmissionPressure { return snapshot },
		})
		if err != nil {
			t.Fatal(err)
		}
		// No saturation; the tagged Standard policy still sheds optional
		// bootstrap calls under live queue/backoff pressure.
		response := httptest.NewRecorder()
		proxy.server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/backend-api/plugins/featured", nil))
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"items":[]`) {
			t.Fatalf("metadata pressure %#v code=%d body=%s", snapshot, response.Code, response.Body.String())
		}
	}
}

func pressureTestRouter04360(t *testing.T) *routingusecase.Router {
	t.Helper()
	router, err := routingusecase.NewRouter(routingusecase.Config{Accounts: func(context.Context) ([]proxymodel.Account, error) {
		return []proxymodel.Account{{ID: "fixture", Enabled: true, Home: "/fixture"}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { router.Close() })
	return router
}
