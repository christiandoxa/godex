package main

import (
	"os"
	"testing"
)

func TestProdex04380IncompleteGatewayOracleRejectsResponseAndReplayMutations(t *testing.T) {
	canonical := incompleteGatewayResult04380{calls: 2, statuses: []int{200, 200},
		contentTypes:          []string{"application/json", "text/event-stream"},
		bodies:                []string{incompleteResponse04380, incompleteSSE04380},
		upstreamAuthenticated: true, exited: true}
	if !sameGatewayIncomplete04380(canonical, canonical) {
		t.Fatal("reference incomplete fixture rejected")
	}
	mutated := []incompleteGatewayResult04380{}
	change := func(edit func(*incompleteGatewayResult04380)) {
		candidate := canonical
		candidate.statuses = append([]int{}, candidate.statuses...)
		candidate.contentTypes = append([]string{}, candidate.contentTypes...)
		candidate.bodies = append([]string{}, candidate.bodies...)
		edit(&candidate)
		mutated = append(mutated, candidate)
	}
	change(func(v *incompleteGatewayResult04380) { v.calls = 3 })
	change(func(v *incompleteGatewayResult04380) { v.statuses[1] = 500 })
	change(func(v *incompleteGatewayResult04380) { v.contentTypes[1] = "application/json" })
	change(func(v *incompleteGatewayResult04380) { v.bodies[0] = `{"status":"completed"}` })
	change(func(v *incompleteGatewayResult04380) { v.bodies[0] += "\n" })
	change(func(v *incompleteGatewayResult04380) { v.bodies[1] = v.bodies[1][:len(v.bodies[1])-2] })
	change(func(v *incompleteGatewayResult04380) { v.upstreamAuthenticated = false })
	change(func(v *incompleteGatewayResult04380) { v.exited = false })
	for i, v := range mutated {
		if sameGatewayIncomplete04380(canonical, v) || sameGatewayIncomplete04380(v, canonical) {
			t.Fatalf("negative control %d escaped incomplete wire oracle: %+v", i, v)
		}
	}
}

// Optional direct binary smoke; the full profileparity command adds pinned
// source SHA and candidate build VCS/clean-tree verification.
func TestProdex04380IncompleteGatewayRealExecutables(t *testing.T) {
	prodex := os.Getenv("GODEX_TEST_PRODEX_04380_BIN")
	godex := os.Getenv("GODEX_TEST_GODEX_04380_BIN")
	if prodex == "" || godex == "" {
		t.Skip("set exact source-verified binary paths to run direct wire fixture")
	}
	phases, err := checkGatewayIncomplete04380(t.TempDir(), cliOptions{prodex: prodex, godex: godex})
	if err != nil || len(phases) != 1 || phases[0].Requests != 2 {
		t.Fatalf("real native incomplete parity diverged: result=%+v err=%v", phases, err)
	}
}
