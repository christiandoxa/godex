package routing

import "testing"

func TestProdex04383HardIdentityReleasePreservesAccompanyingSession(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys affinityKeys
		keep bool
	}{
		{"owned turn", affinityKeys{turn: "turn-a", session: "session-a"}, true},
		{"missing turn", affinityKeys{turn: "missing", session: "session-a"}, true},
		{"owned response", affinityKeys{previous: "resp-a", session: "session-a"}, true},
		{"missing response", affinityKeys{previous: "missing", session: "session-a"}, true},
		{"session alone", affinityKeys{session: "session-a"}, false},
		{"blank hard identity", affinityKeys{turn: " \t", previous: " \n", session: "session-a"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newAffinityStore()
			now := store.clock()
			if err := store.remember(t.Context(), "owner-a", affinityKeys{turn: "turn-a", previous: "resp-a", session: "session-a"}, now); err != nil {
				t.Fatal(err)
			}
			if err := store.releaseOwnedDead(t.Context(), "owner-a", tc.keys, now); err != nil {
				t.Fatal(err)
			}
			owner, err := store.owner(t.Context(), affinityKeys{session: "session-a"}, now)
			if err != nil || (owner == "owner-a") != tc.keep {
				t.Fatalf("session owner=%q err=%v, retain=%t", owner, err, tc.keep)
			}
			for _, keys := range []affinityKeys{{turn: "turn-a"}, {previous: "resp-a"}} {
				owner, err := store.owner(t.Context(), keys, now)
				released := (keys.turn != "" && keys.turn == tc.keys.turn) || (keys.previous != "" && keys.previous == tc.keys.previous)
				if err != nil || (owner == "") != released {
					t.Fatalf("hard owner=%q err=%v, release=%t", owner, err, released)
				}
			}
		})
	}
}
