package main

import (
	"strings"
	"testing"
)

// The actual pinned 0.438.4 executable replays a reconstructable full history
// from its old Compact turn owner to an authenticated Ready alternate. It
// scrubs both stale turn-state carriers without losing opaque user history.
func TestProdex04383CompactBoundTransportRejectsReplayMutations(t *testing.T) {
	old := transportAttempt04383{owner: "account-b", ready: true, turn: "compact-turn", body: compactLineageMessage04383}
	clean := strings.Replace(compactLineageMessage04383, `"x-codex-turn-state":"compact-turn",`, "", 1)
	alternate := transportAttempt04383{owner: "account-a", ready: true, body: clean}
	frames := []string{transportDelta04383, transportCompleted04383}
	if err := verifyCompactBoundTransport04383([]string{"account-b", "account-a"}, []transportAttempt04383{old, alternate}, false, frames); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		handshakes []string
		attempts   []transportAttempt04383
		invalid    bool
		frames     []string
	}{
		{"no alternate", []string{"account-b"}, []transportAttempt04383{old}, false, frames},
		{"third replay", []string{"account-b", "account-a", "account-b"}, []transportAttempt04383{old, alternate, old}, false, frames},
		{"wrong first owner", []string{"account-a", "account-a"}, []transportAttempt04383{old, alternate}, false, frames},
		{"stale header on alternate", []string{"account-b", "account-a"}, []transportAttempt04383{old, {owner: "account-a", ready: true, turn: "compact-turn", body: clean}}, false, frames},
		{"stale body turn on alternate", []string{"account-b", "account-a"}, []transportAttempt04383{old, {owner: "account-a", ready: true, body: compactLineageMessage04383}}, false, frames},
		{"missing Ready", []string{"account-b", "account-a"}, []transportAttempt04383{old, {owner: "account-a", body: clean}}, false, frames},
		{"lost original turn", []string{"account-b", "account-a"}, []transportAttempt04383{{owner: "account-b", ready: true, body: compactLineageMessage04383}, alternate}, false, frames},
		{"corrupt full history", []string{"account-b", "account-a"}, []transportAttempt04383{old, {owner: "account-a", ready: true, body: strings.ReplaceAll(clean, "followup", "lost")}}, false, frames},
		{"early success", []string{"account-b", "account-a"}, []transportAttempt04383{old, alternate}, false, []string{transportCompleted04383}},
		{"wrong terminal response", []string{"account-b", "account-a"}, []transportAttempt04383{old, alternate}, false, []string{transportDelta04383, strings.ReplaceAll(transportCompleted04383, "resp-transport-recovered", "wrong")}},
		{"unauthenticated", []string{"account-b", "account-a"}, []transportAttempt04383{old, alternate}, true, frames},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := verifyCompactBoundTransport04383(tc.handshakes, tc.attempts, tc.invalid, tc.frames); err == nil {
				t.Fatalf("Compact full-history replay mutation escaped: %s", tc.name)
			}
		})
	}
}
