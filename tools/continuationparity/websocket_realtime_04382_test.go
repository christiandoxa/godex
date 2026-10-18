package main

import "testing"

func TestProdex04382RealtimeOracleRejectsMutations(t *testing.T) {
	valid := realtimeResult04382{
		owner: "account-a", handshakes: 1, controls: 2, audio: 1, turns: 1,
		closed: 1, waiting: true, responseHandshakes: 1,
		attempts: []attempt{{account: "account-a", body: capacityRequestBody04382}},
	}
	if err := verifyRealtimeResult04382(valid, "client"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*realtimeResult04382)
	}{
		{"owner rotation", func(v *realtimeResult04382) { v.owner = "account-b" }},
		{"extra live handshake", func(v *realtimeResult04382) { v.handshakes++ }},
		{"missing control", func(v *realtimeResult04382) { v.controls-- }},
		{"modified realtime payload or auth", func(v *realtimeResult04382) { v.wireChanged = true }},
		{"lost audio", func(v *realtimeResult04382) { v.audio-- }},
		{"duplicate live inference", func(v *realtimeResult04382) { v.turns++ }},
		{"permit retained", func(v *realtimeResult04382) { v.closed = 0 }},
		{"duplicate close", func(v *realtimeResult04382) { v.closed++ }},
		{"premature dispatch", func(v *realtimeResult04382) { v.waiting = false }},
		{"reconnected follower", func(v *realtimeResult04382) { v.responseHandshakes++ }},
		{"lost waiting turn", func(v *realtimeResult04382) { v.attempts = nil }},
		{"duplicate waiting inference", func(v *realtimeResult04382) { v.attempts = append(v.attempts, v.attempts[0]) }},
		{"waiting owner rotated", func(v *realtimeResult04382) { v.attempts[0].account = "account-b" }},
		{"waiting request mutated", func(v *realtimeResult04382) { v.attempts[0].body = `{}` }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := valid
			result.attempts = append([]attempt(nil), valid.attempts...)
			tc.mutate(&result)
			if err := verifyRealtimeResult04382(result, "client"); err == nil {
				t.Fatal("realtime mutation escaped oracle")
			}
		})
	}
	valid.responseHandshakes, valid.attempts = 0, nil
	if err := verifyRealtimeResult04382(valid, "shutdown"); err != nil {
		t.Fatal(err)
	}
	valid.attempts = []attempt{{account: "account-a", body: capacityRequestBody04382}}
	if err := verifyRealtimeResult04382(valid, "shutdown"); err == nil {
		t.Fatal("shutdown replay escaped oracle")
	}
}
