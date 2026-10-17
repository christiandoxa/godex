package main

import (
	"testing"
	"time"
)

func TestProdex04382WebSocketCommittedIdleOracleRejectsWireMutations(t *testing.T) {
	good := websocketCommittedIdleResult04382{
		eventType: "response.output_text.delta", delta: "visible-idle",
		closeOpcode: 8, elapsed: 2 * time.Second, handshakes: 1,
		attempts: []attempt{{account: "account-a", mode: committedIdleMode04382, body: committedIdleBody04382}},
	}
	if err := verifyWebSocketCommittedIdle04382(good); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*websocketCommittedIdleResult04382)
	}{
		{"lost visible delta", func(r *websocketCommittedIdleResult04382) { r.delta = "" }},
		{"changed frame type", func(r *websocketCommittedIdleResult04382) { r.eventType = "response.created" }},
		{"abrupt TCP end instead of close frame", func(r *websocketCommittedIdleResult04382) { r.closeOpcode = 0 }},
		{"close with foreign payload", func(r *websocketCommittedIdleResult04382) { r.closePayload = []byte{3, 232} }},
		{"incomplete response masqueraded as close", func(r *websocketCommittedIdleResult04382) { r.closeOpcode = 1 }},
		{"expired before configured idle", func(r *websocketCommittedIdleResult04382) { r.elapsed = 100 * time.Millisecond }},
		{"old five minute idle window", func(r *websocketCommittedIdleResult04382) { r.elapsed = 5 * time.Minute }},
		{"unexpected extra connection", func(r *websocketCommittedIdleResult04382) { r.handshakes = 2 }},
		{"no initial upstream dispatch", func(r *websocketCommittedIdleResult04382) { r.attempts = nil }},
		{"postcommit retry replay", func(r *websocketCommittedIdleResult04382) { r.attempts = append(r.attempts, r.attempts[0]) }},
		{"wrong owner", func(r *websocketCommittedIdleResult04382) { r.attempts[0].account = "account-b" }},
		{"rewritten request JSON", func(r *websocketCommittedIdleResult04382) { r.attempts[0].body = `{"type":"response.create"}` }},
		{"different mode", func(r *websocketCommittedIdleResult04382) { r.attempts[0].mode = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			altered := good
			altered.attempts = append([]attempt(nil), good.attempts...)
			tc.mutate(&altered)
			if err := verifyWebSocketCommittedIdle04382(altered); err == nil {
				t.Fatalf("committed idle mutation %q evaded exact two-binary oracle", tc.name)
			}
		})
	}
}
