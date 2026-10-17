package main

import (
	"strings"
	"testing"
	"time"
)

func TestProdex04382LookaheadIdleOracleRejectsMutations(t *testing.T) {
	baseline := websocketLookaheadIdleResult04382{
		eventType: "response.in_progress", padding: strings.Repeat("x", lookaheadPadding04382),
		closeOpcode: 8, elapsed: 2 * time.Second, handshakes: 1,
		attempts: []attempt{{account: "account-a", mode: lookaheadIdleMode04382, body: lookaheadIdleBody04382}},
	}
	if err := verifyWebSocketLookaheadIdle04382(baseline); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*websocketLookaheadIdleResult04382)
	}{
		{"hidden content lost", func(r *websocketLookaheadIdleResult04382) { r.padding = "" }},
		{"wrong hold size", func(r *websocketLookaheadIdleResult04382) { r.padding = r.padding[:4096] }},
		{"wrong hold kind", func(r *websocketLookaheadIdleResult04382) { r.eventType = "response.completed" }},
		{"abrupt TCP end", func(r *websocketLookaheadIdleResult04382) { r.closeOpcode = 0 }},
		{"close reason drift", func(r *websocketLookaheadIdleResult04382) { r.closePayload = []byte{3, 232} }},
		{"old eight second watchdog", func(r *websocketLookaheadIdleResult04382) { r.elapsed = 8 * time.Second }},
		{"premature timeout", func(r *websocketLookaheadIdleResult04382) { r.elapsed = 500 * time.Millisecond }},
		{"duplicated inference", func(r *websocketLookaheadIdleResult04382) { r.attempts = append(r.attempts, r.attempts[0]) }},
		{"new handshake", func(r *websocketLookaheadIdleResult04382) { r.handshakes = 2 }},
		{"wrong owner", func(r *websocketLookaheadIdleResult04382) { r.attempts[0].account = "account-b" }},
		{"rewritten request body", func(r *websocketLookaheadIdleResult04382) { r.attempts[0].body = `{"type":"response.create"}` }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := baseline
			got.attempts = append([]attempt(nil), baseline.attempts...)
			tc.mutate(&got)
			if err := verifyWebSocketLookaheadIdle04382(got); err == nil {
				t.Fatalf("tagged lookahead mutation %q evaded oracle", tc.name)
			}
		})
	}
}
