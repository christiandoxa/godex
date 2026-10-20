package main

import (
	"strings"
	"testing"
)

func TestProdex04383WebSocketCompactLineageOracleRejectsMutations(t *testing.T) {
	for _, expected := range []string{"main", "second"} {
		owner := "account-a"
		if expected == "second" {
			owner = "account-b"
		}
		attempt := transportAttempt04383{owner: owner, turn: "compact-turn", ready: true, body: compactLineageMessage04383}
		hello := []string{owner}
		frames := []string{transportDelta04383, transportCompleted04383}
		if err := verifyCompactLineage04383(expected, hello, []transportAttempt04383{attempt}, frames, false); err != nil {
			t.Fatalf("valid %s Compact lineage failed: %v", expected, err)
		}
		for _, mutation := range []struct {
			name     string
			hello    []string
			attempts []transportAttempt04383
			frames   []string
			invalid  bool
		}{
			{"missing readiness", hello, []transportAttempt04383{{owner: owner, body: compactLineageMessage04383}}, frames, false},
			{"wrong owner", []string{"account-a", "account-b"}, []transportAttempt04383{attempt}, frames, false},
			{"stale turn header", hello, []transportAttempt04383{{owner: owner, turn: "foreign-turn", ready: true, body: compactLineageMessage04383}}, frames, false},
			{"changed opaque history", hello, []transportAttempt04383{{owner: owner, turn: "compact-turn", ready: true, body: strings.ReplaceAll(compactLineageMessage04383, `"content":"answer"`, `"content":"changed"`)}}, frames, false},
			{"duplicated model request", hello, []transportAttempt04383{attempt, attempt}, frames, false},
			{"changed client output", hello, []transportAttempt04383{attempt}, []string{`{"type":"response.output_text.delta","delta":"wrong"}`, transportCompleted04383}, false},
			{"lost terminal", hello, []transportAttempt04383{attempt}, []string{transportDelta04383}, false},
			{"invalid credentials", hello, []transportAttempt04383{attempt}, frames, true},
		} {
			t.Run(expected+"/"+mutation.name, func(t *testing.T) {
				if err := verifyCompactLineage04383(expected, mutation.hello, mutation.attempts, mutation.frames, mutation.invalid); err == nil {
					t.Fatalf("Compact lineage oracle accepted %s mutation", mutation.name)
				}
			})
		}
	}
}
