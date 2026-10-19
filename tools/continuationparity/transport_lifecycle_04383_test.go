package main

import (
	"testing"
	"time"
)

func TestProdex04383TransportLifecycleOracleRejectsMutations(t *testing.T) {
	for _, scenario := range []string{"deadline", "cancellation", "postcommit"} {
		t.Run(scenario, func(t *testing.T) {
			base := transportLifecycleResult04383{
				scenario: scenario, code: "request_timeout", status: 504, message: "request timed out", elapsed: 30 * time.Second, heartbeats: 25,
				canceled: true, termination: "tcp-eof", handshakes: []string{"account-a"},
				attempts: []transportAttempt04383{{owner: "account-a", turn: "turn-a", body: transportHistory04383, ready: true}},
				frames:   []string{`{"type":"response.created","response":{"id":"discarded-transport"}}`, `{"type":"response.output_text.delta","delta":"visible"}`},
			}
			if err := verifyTransportLifecycle04383(base); err != nil {
				t.Fatal(err)
			}
			mutations := map[string]func(*transportLifecycleResult04383){
				"replay":        func(r *transportLifecycleResult04383) { r.attempts = append(r.attempts, r.attempts[0]) },
				"owner":         func(r *transportLifecycleResult04383) { r.attempts[0].owner = "account-b" },
				"missing Ready": func(r *transportLifecycleResult04383) { r.attempts[0].ready = false },
				"changed body":  func(r *transportLifecycleResult04383) { r.attempts[0].body = `{"input":[]}` },
			}
			switch scenario {
			case "deadline":
				mutations["extended deadline"] = func(r *transportLifecycleResult04383) { r.elapsed = 32 * time.Second }
				mutations["early deadline"] = func(r *transportLifecycleResult04383) { r.elapsed = 25 * time.Second }
				mutations["wrong error"] = func(r *transportLifecycleResult04383) { r.status = 503 }
				mutations["no liveness"] = func(r *transportLifecycleResult04383) { r.heartbeats = 0 }
			case "cancellation":
				mutations["upstream remains live"] = func(r *transportLifecycleResult04383) { r.canceled = false }
			case "postcommit":
				mutations["lost output"] = func(r *transportLifecycleResult04383) { r.frames = r.frames[:1] }
				mutations["truncated frame"] = func(r *transportLifecycleResult04383) { r.termination = "partial-eof" }
				mutations["changed close"] = func(r *transportLifecycleResult04383) { r.termination = "empty-close" }
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					got := base
					got.attempts = append([]transportAttempt04383(nil), base.attempts...)
					mutate(&got)
					if err := verifyTransportLifecycle04383(got); err == nil {
						t.Fatal("lifecycle mutation escaped the oracle")
					}
				})
			}
		})
	}
}
