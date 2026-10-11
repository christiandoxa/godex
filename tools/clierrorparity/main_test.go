package main

import "testing"

func TestCompareProbeDetectsAcceptedNegativeControl(t *testing.T) {
	item := probe{Name: "current", Arguments: []string{"current", "extra"}, Rejects: true}
	canonical := commandResult{ExitCode: 2, Output: "unexpected argument"}
	accepted := commandResult{Accepted: true, Output: "No active profile."}
	if differences := compareProbe(item, canonical, accepted); len(differences) == 0 {
		t.Fatal("accepted invalid argument escaped the oracle")
	}
}

func TestCompareProbeAcceptsMatchingOutcomes(t *testing.T) {
	item := probe{Name: "current", Arguments: []string{"current"}}
	accepted := commandResult{Accepted: true, Output: "No active profile."}
	if differences := compareProbe(item, accepted, accepted); len(differences) != 0 {
		t.Fatalf("matching valid command rejected: %v", differences)
	}

	item.Rejects = true
	item.Arguments = []string{"current", "extra"}
	rejected := commandResult{ExitCode: 2, Output: "unexpected argument"}
	if differences := compareProbe(item, rejected, rejected); len(differences) != 0 {
		t.Fatalf("matching rejected command rejected: %v", differences)
	}
}

func TestNegativeControlPasses(t *testing.T) {
	if err := checkNegativeControl(); err != nil {
		t.Fatal(err)
	}
}
