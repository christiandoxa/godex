package routing

import (
	"testing"
	"time"
)

func TestRouteCircuitOpenEscalationAndHalfOpenProbe(t *testing.T) {
	now := time.Unix(100, 0)
	first, opened := OpenRouteCircuit("account-a", "responses", 4, nil, now)
	if !opened || first.UntilUnix != now.Add(20*time.Second).Unix() || first.ReopenStage != 0 {
		t.Fatalf("first circuit = %+v, opened = %t", first, opened)
	}
	second, opened := OpenRouteCircuit("account-a", "responses", 5, &first, now.Add(time.Second))
	if !opened || second.UntilUnix != now.Add(81*time.Second).Unix() || second.ReopenStage != 1 {
		t.Fatalf("reopened circuit = %+v, opened = %t", second, opened)
	}
	if got := second.Remaining(now.Add(time.Second)); got != 80*time.Second {
		t.Fatalf("reopened duration = %s, want 80s", got)
	}

	probe, allowed, changed := second.ReserveProbe(5, time.Unix(second.UntilUnix, 0))
	if !allowed || !changed || probe.UntilUnix != second.UntilUnix+10 {
		t.Fatalf("half-open probe = %+v, allowed = %t, changed = %t", probe, allowed, changed)
	}
	if _, allowed, changed := probe.ReserveProbe(5, time.Unix(second.UntilUnix, 0)); allowed || changed {
		t.Fatalf("concurrent half-open probe allowed = %t, changed = %t", allowed, changed)
	}
}

func TestRouteCircuitBoundsStartupSofteningAndReopenStage(t *testing.T) {
	now := time.Unix(100, 0)
	previous := &RouteCircuit{UntilUnix: now.Add(time.Second).Unix(), ReopenStage: RouteCircuitMaxReopenStage, StageUpdatedUnix: now.Unix()}
	circuit, opened := OpenRouteCircuit("account-a", "compact", 16, previous, now)
	if !opened || circuit.UntilUnix != now.Add(RouteCircuitMaxOpenDuration).Unix() || circuit.ReopenStage != RouteCircuitMaxReopenStage {
		t.Fatalf("maximum circuit = %+v, opened = %t", circuit, opened)
	}
	softened, keep, changed := circuit.SoftenForRestart(16, now)
	if !keep || !changed || softened.UntilUnix != now.Add(40*time.Second).Unix() {
		t.Fatalf("softened circuit = %+v, keep = %t, changed = %t", softened, keep, changed)
	}
	if _, keep, changed := circuit.SoftenForRestart(16, time.Unix(circuit.UntilUnix, 0)); keep || !changed {
		t.Fatalf("expired circuit retained = %t, changed = %t", keep, changed)
	}
	if _, opened := OpenRouteCircuit("account-a", "compact", 3, nil, now); opened {
		t.Fatal("circuit opened below health threshold")
	}
}
