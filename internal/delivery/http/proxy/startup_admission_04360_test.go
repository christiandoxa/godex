package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// The exact Prodex 0.436.0 standard handler returns a synthetic response
// for optional startup metadata once standard admissions reach half its
// lane limit, while important model/MCP startup requests retain priority.
func TestProdex04360OptionalStartupMetadataShedsAtHalfStandardLane(t *testing.T) {
	cases := []struct {
		name, path string
		wantCode   int
		wantBody   string
	}{
		{"featured", "/backend-api/plugins/featured?platform=codex", 200, `{"items":[],"data":[]}`},
		{"installed", "/backend-api/ps/plugins/installed", 200, `{"items":[],"data":[]}`},
		{"directory", "/backend-api/connectors/directory/list", 200, `{"items":[],"data":[]}`},
		{"analytics", "/backend-api/godex/analytics-events/events", 204, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writer.WriteHeader(http.StatusTeapot)
			}), admissionLimits{global: 20, lane: [admissionLaneCount]int{20, 20, 20, 12}}).(*activeRequestHandler)
			handler.active = 6
			handler.laneActive[admissionLaneStandard] = 6
			writer := httptest.NewRecorder()
			handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if writer.Code != tc.wantCode || strings.TrimSpace(writer.Body.String()) != tc.wantBody || calls.Load() != 0 {
				t.Fatalf("response=%d body=%q forwarded=%d", writer.Code, writer.Body.String(), calls.Load())
			}
			if tc.wantCode == 200 && writer.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("missing JSON content type: %#v", writer.Header())
			}
			if handler.active != 6 || handler.laneActive[admissionLaneStandard] != 6 {
				t.Fatalf("synthetic metadata response leaked admission capacity: %d/%d", handler.active, handler.laneActive[admissionLaneStandard])
			}
		})
	}
}

func TestProdex04360StartupMetadataNotShedBelowHalfOrForRequiredRoutes(t *testing.T) {
	for _, tc := range []struct {
		path   string
		active int
	}{
		{"/backend-api/codex/plugins/featured", 5},
		{"/backend-api/prodex/v0.436.0/plugins/featured", 5},
		{"/backend-api/prodex/plugins/featured", 6}, // Prodex normalizes this to /backend-api/codex/plugins/featured, not an optional path.
		{"/backend-api/codex/models", 6},
		{"/backend-api/ps/mcp", 6},
		{"/backend-api/codex/responses/compact", 6},
	} {
		var calls atomic.Int32
		handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			_, _ = io.WriteString(w, "upstream")
		}), admissionLimits{global: 20, lane: [admissionLaneCount]int{20, 20, 20, 12}}).(*activeRequestHandler)
		handler.active = tc.active
		handler.laneActive[admissionLaneStandard] = tc.active
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != 200 || response.Body.String() != "upstream" || calls.Load() != 1 {
			t.Fatalf("%s (standard active %d) code=%d body=%q forwardCount=%d", tc.path, tc.active, response.Code, response.Body.String(), calls.Load())
		}
	}
}

func TestProdex04360StartupModelAndMCPBypassStandardLaneOnly(t *testing.T) {
	for _, path := range []string{
		"/backend-api/codex/models?client_version=0.161.0",
		"/backend-api/prodex/models",
		"/backend-api/godex/models",
		"/backend-api/ps/mcp",
	} {
		var calls atomic.Int32
		handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(209)
		}), admissionLimits{global: 3, lane: [admissionLaneCount]int{3, 3, 3, 1}}).(*activeRequestHandler)
		handler.active = 1
		handler.laneActive[admissionLaneStandard] = 1
		handler.wait = func(context.Context, <-chan struct{}) bool { return false }
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if calls.Load() != 1 || recorder.Code != 209 || handler.active != 1 || handler.laneActive[admissionLaneStandard] != 1 {
			t.Fatalf("startup route %q blocked under lane pressure: calls=%d code=%d inFlight=%d/%d", path, calls.Load(), recorder.Code, handler.active, handler.laneActive[admissionLaneStandard])
		}
	}
}

func TestProdex04360StartupPriorityDoesNotBypassGlobalLimit(t *testing.T) {
	var calls atomic.Int32
	handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }),
		admissionLimits{global: 1, lane: [admissionLaneCount]int{1, 1, 1, 1}}).(*activeRequestHandler)
	handler.active = 1
	handler.laneActive[admissionLaneStandard] = 1
	handler.wait = func(context.Context, <-chan struct{}) bool { return false }
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/backend-api/codex/models", nil))
	if calls.Load() != 0 || handler.active != 1 {
		t.Fatalf("priority bypassed global cap")
	}
}
