package proxy

import (
	"context"
	"net/http"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type admissionMarkerRecorder struct{ events []runtimemodel.Event }

func (recorder *admissionMarkerRecorder) Record(_ context.Context, event runtimemodel.Event) error {
	recorder.events = append(recorder.events, event)
	return nil
}

func TestProdex04355AdmissionRecordsGlobalAndLanePressureMarkers(t *testing.T) {
	for _, test := range []struct {
		name       string
		active     int
		laneActive int
		wantKind   string
		wantActive string
		wantLimit  string
	}{
		{name: "global", active: 4, laneActive: 1, wantKind: "runtime_proxy_active_limit_reached", wantActive: "4", wantLimit: "4"},
		{name: "lane", active: 2, laneActive: 2, wantKind: "runtime_proxy_lane_limit_reached", wantActive: "2", wantLimit: "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &admissionMarkerRecorder{}
			handler := newActiveRequestHandlerWithLimitsAndRecorder(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), admissionLimits{
				global: 4, lane: [admissionLaneCount]int{4, 2, 4, 4},
			}, recorder).(*activeRequestHandler)
			handler.active = test.active
			handler.laneActive[admissionLaneCompact] = test.laneActive
			handler.wait = func(context.Context, <-chan struct{}) bool { return false }
			if handler.acquireWithMetadata(context.Background(), admissionLaneCompact, "/responses/compact", "http") {
				t.Fatal("saturated admission unexpectedly acquired")
			}
			if len(recorder.events) != 1 {
				t.Fatalf("events = %#v", recorder.events)
			}
			event := recorder.events[0]
			if event.Kind != test.wantKind || event.Fields["active"] != test.wantActive || event.Fields["limit"] != test.wantLimit ||
				event.Fields["lane"] != "compact" || event.Fields["transport"] != "http" || event.Fields["path"] != "/responses/compact" {
				t.Fatalf("pressure event = %#v", event)
			}
		})
	}
}
