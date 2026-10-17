package main

import (
	"testing"
	"time"
)

// Negative sentinels for the independently pinned Prodex 0.438.2 websocket
// deadline fixture. An incomplete read, early timeout, second hidden turn,
// owner rotation, handshake leak or altered status/message must all fail.
func TestProdex04382WebSocketDeadlineOracleRejectsMaterialMutations(t *testing.T) {
	base := websocketDeadlineResult04382{
		code: "request_timeout", status: 504, message: deadlineMessage04382,
		elapsed: 30 * time.Second, completed: "resp-after-watchdog", handshakes: 2,
		attempts: []attempt{
			{account: "account-a", mode: deadlineMode04382, body: `{"type":"response.create","mode":"ws-hidden-deadline-04382","input":[]}`},
			{account: "account-a", mode: deadlineFollowupMode04382, body: `{"type":"response.create","mode":"ws-followup-deadline-04382","input":[]}`},
		},
	}
	if err := verifyWebSocketDeadline04382(base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*websocketDeadlineResult04382)
	}{
		{"missing status", func(r *websocketDeadlineResult04382) { r.status = 0 }},
		{"quota error masquerade", func(r *websocketDeadlineResult04382) { r.code = "insufficient_quota" }},
		{"error message drift", func(r *websocketDeadlineResult04382) { r.message = "upstream timed out" }},
		{"renewed watchdog", func(r *websocketDeadlineResult04382) { r.elapsed = 32 * time.Second }},
		{"early false timeout", func(r *websocketDeadlineResult04382) { r.elapsed = 25 * time.Second }},
		{"reconnected local owner", func(r *websocketDeadlineResult04382) { r.handshakes = 3 }},
		{"missing upstream handshake", func(r *websocketDeadlineResult04382) { r.handshakes = 1 }},
		{"missing followup", func(r *websocketDeadlineResult04382) { r.attempts = r.attempts[:1] }},
		{"duplicate timeout dispatch", func(r *websocketDeadlineResult04382) { r.attempts = append(r.attempts, r.attempts[0]) }},
		{"owner rotated", func(r *websocketDeadlineResult04382) { r.attempts[0].account = "account-b" }},
		{"followup owner rotated", func(r *websocketDeadlineResult04382) { r.attempts[1].account = "account-b" }},
		{"followup request body changed", func(r *websocketDeadlineResult04382) { r.attempts[1].body = `{"type":"response.create"}` }},
		{"missing completion identity", func(r *websocketDeadlineResult04382) { r.completed = "" }},
		{"wrong completion identity", func(r *websocketDeadlineResult04382) { r.completed = "resp-different" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := base
			got.attempts = append([]attempt(nil), base.attempts...)
			tc.mutate(&got)
			if err := verifyWebSocketDeadline04382(got); err == nil {
				t.Fatalf("0.438.2 precommit mutation %q evaded differential checker", tc.name)
			}
		})
	}
}
