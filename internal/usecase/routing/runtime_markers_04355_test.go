package routing

import (
	"context"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type routingMarkerRecorder struct{ events []runtimemodel.Event }

func (recorder *routingMarkerRecorder) Record(_ context.Context, event runtimemodel.Event) error {
	recorder.events = append(recorder.events, event)
	return nil
}

func TestProdex04355ProfileInflightSaturationRecordsRuntimeMarker(t *testing.T) {
	recorder := &routingMarkerRecorder{}
	router, err := NewRouter(Config{
		Activity: recorder, ProfileInflightHardLimit: 2,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "main", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := proxymodel.Request{RequestID: 44, Path: "/responses", Header: make(http.Header), QuotaSelection: quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}}
	release, ok := router.tryAcquireProfileInflight("main", request, false)
	if !ok {
		t.Fatal("first permit rejected")
	}
	defer release()
	if _, ok := router.tryAcquireProfileInflight("main", request, false); ok {
		t.Fatal("saturated permit acquired")
	}
	if len(recorder.events) != 1 {
		t.Fatalf("events = %#v", recorder.events)
	}
	event := recorder.events[0]
	if event.Kind != "profile_inflight_saturated" || event.RequestID != "44" || event.Fields["profile"] != "main" ||
		event.Fields["hard_limit"] != "2" || event.Fields["route"] != "responses" || event.Fields["transport"] != "http" {
		t.Fatalf("profile marker = %#v", event)
	}
}

func TestProdex04355RouteHealthPenaltyRecordsScopedRuntimeMarker(t *testing.T) {
	recorder := &routingMarkerRecorder{}
	router, err := NewRouter(Config{
		Activity: recorder,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return []proxymodel.Account{{ID: "alpha", Enabled: true}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	router.recordRouteFailurePenalty(context.Background(), "alpha", quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, 2)
	if len(recorder.events) != 1 {
		t.Fatalf("events = %#v", recorder.events)
	}
	event := recorder.events[0]
	if event.Kind != "profile_health" || event.Fields["profile"] != "alpha" || event.Fields["route"] != "responses" ||
		event.Fields["score"] != "2" || event.Fields["delta"] != "2" || event.Fields["reason"] != "responses_overload" {
		t.Fatalf("health marker = %#v", event)
	}
}
