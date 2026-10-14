package runtime

import (
	"context"
	"testing"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestProdex043711DoctorRouteHealthExposesHalfOpenCircuitAfterCooldown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	state := &doctorRouteHealthState{
		now: now,
		circuits: []routingentity.RouteCircuit{
			{
				AccountID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Route:            "responses",
				UntilUnix:        now.Unix() - 1,
				StageUpdatedUnix: now.Unix() - 40,
			},
			{
				AccountID:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Route:            "websocket",
				UntilUnix:        now.Unix() + 10,
				StageUpdatedUnix: now.Unix() - 5,
			},
		},
	}
	got := doctorRouteHealthWithState(nil, state)
	if len(got) != 2 {
		t.Fatalf("doctor circuit groups=%d, want two: %+v", len(got), got)
	}
	if got[0].CircuitState != "half-open" || got[0].CircuitUntil == nil ||
		*got[0].CircuitUntil != now.Unix()-1 {
		t.Fatalf("expired half-open doctor circuit lost: %+v", got[0])
	}
	if got[1].CircuitState != "open" || got[1].CircuitUntil == nil ||
		*got[1].CircuitUntil != now.Unix()+10 {
		t.Fatalf("open doctor circuit lost: %+v", got[1])
	}
}

type doctorCircuitInspectionFixture043711 struct {
	circuit          routingentity.RouteCircuit
	inspectionCalls  int
	routingLoadCalls int
}

func (*doctorCircuitInspectionFixture043711) LoadRouteHealth(context.Context, time.Time) ([]routingentity.RouteHealthScore, error) {
	return nil, nil
}
func (*doctorCircuitInspectionFixture043711) LoadRouteMemory(context.Context, time.Time) ([]routingentity.RouteMemoryScore, error) {
	return nil, nil
}
func (fixture *doctorCircuitInspectionFixture043711) LoadRouteCircuits(context.Context, time.Time, []routingentity.RouteHealthScore) ([]routingentity.RouteCircuit, error) {
	fixture.routingLoadCalls++
	return nil, nil
}
func (*doctorCircuitInspectionFixture043711) LoadTransportBackoffs(context.Context, time.Time) ([]routingentity.TransportBackoff, error) {
	return nil, nil
}
func (fixture *doctorCircuitInspectionFixture043711) InspectRouteCircuits(context.Context, time.Time) ([]routingentity.RouteCircuit, error) {
	fixture.inspectionCalls++
	return []routingentity.RouteCircuit{fixture.circuit}, nil
}

func TestProdex043711DoctorDiagnoseDoesNotMutateCircuitOrLoseHalfOpen(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	state := &doctorCircuitInspectionFixture043711{circuit: routingentity.RouteCircuit{
		AccountID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Route:     "responses", UntilUnix: now.Unix() - 1,
		StageUpdatedUnix: now.Unix() - 20,
	}}
	doctor := NewDoctor(&fakeDoctorAccounts{}, fakeVersionedCodex{})
	doctor.now = func() time.Time { return now }
	doctor.SetActivity(fakeDoctorActivity{})
	doctor.SetRouteHealthState(state)
	report, err := doctor.Diagnose(t.Context(), runtimemodel.DoctorOptions{Runtime: true, TailBytes: 1024})
	if err != nil || report.Runtime == nil {
		t.Fatalf("diagnose: %+v %v", report, err)
	}
	if state.inspectionCalls != 1 || state.routingLoadCalls != 0 {
		t.Fatalf("doctor modified routing state instead of observing it: inspections=%d routing_load=%d",
			state.inspectionCalls, state.routingLoadCalls)
	}
	routes := report.Runtime.RouteHealth
	if len(routes) != 1 || routes[0].CircuitState != "half-open" ||
		routes[0].CircuitUntil == nil || *routes[0].CircuitUntil != now.Unix()-1 {
		t.Fatalf("doctor lost tagged half-open state: %+v", routes)
	}
}
