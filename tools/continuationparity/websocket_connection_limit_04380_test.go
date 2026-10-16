package main

import "testing"

// Official Prodex 0.438.0 sends the same precommit response.create to the
// same credential on a new TCP/WebSocket after either terminal-frame limit
// location, without leaking the first terminal error to the client.
func TestProdex04380FreshConnectionLimitOracleRejectsOwnerAndReplayDrift(t *testing.T) {
	for _, tc := range []struct{ mode, completed string }{
		{"ws-limit-top", `{"type":"response.completed","response":{"id":"resp-limit-top-recovered"}}`},
		{"ws-limit-nested", `{"type":"response.completed","response":{"id":"resp-limit-nested-recovered"}}`},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			expected := websocketExpectation{mode: tc.mode, want: []string{"response.completed"}, attempts: 2,
				owner: "account-a", rawFrames: []string{tc.completed}}
			successful := wsProbe{types: []string{"response.completed"}, body: []string{tc.completed}}
			owners := []string{"account-a", "account-a"}
			if err := verifyWebsocketExpectation(expected, successful, owners); err != nil {
				t.Fatal(err)
			}
			fail := func(name string, got wsProbe, attempts []string) {
				t.Helper()
				if err := verifyWebsocketExpectation(expected, got, attempts); err == nil {
					t.Fatalf("%s escaped fresh connection-limit oracle", name)
				}
			}
			fail("never retried", successful, []string{"account-a"})
			fail("third retry", successful, []string{"account-a", "account-a", "account-a"})
			fail("rotated owner", successful, []string{"account-a", "account-b"})
			fail("leaked first error", wsProbe{types: []string{"error"}, body: []string{`{"type":"error","code":"websocket_connection_limit_reached"}`}}, owners)
			fail("changed response ID", wsProbe{types: successful.types, body: []string{`{"type":"response.completed","response":{"id":"resp-different"}}`}}, owners)
			fail("extra completed frame", wsProbe{types: []string{"response.completed", "response.completed"}, body: []string{tc.completed, tc.completed}}, owners)
		})
	}
}
