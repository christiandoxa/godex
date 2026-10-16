package main

import "testing"

func TestProdex04380WebSocketStatusPrecedenceOracleRejectsPrematureTerminal(t *testing.T) {
	frames := []string{
		`{"type":"error","status":"invalid","status_code":400}`,
		`{"type":"response.completed","response":{"id":"resp-status-priority"}}`,
	}
	expected := websocketExpectation{mode: "ws-status-priority", want: []string{"error", "response.completed"},
		attempts: 1, owner: "account-a", rawFrames: frames}
	good := wsProbe{types: []string{"error", "response.completed"}, delta: []string{"", ""}, body: frames}
	if err := verifyWebsocketExpectation(expected, good, []string{"account-a"}); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct {
		name   string
		value  wsProbe
		owners []string
	}{
		{"premature error close", wsProbe{types: []string{"error"}, body: []string{frames[0]}}, []string{"account-a"}},
		{"missing completion", wsProbe{types: good.types, body: frames[:1]}, []string{"account-a"}},
		{"changed status precedence", wsProbe{types: good.types, body: []string{`{"type":"error","status":400,"status_code":400}`, frames[1]}}, []string{"account-a"}},
		{"duplicate attempt", good, []string{"account-a", "account-a"}},
		{"wrong owner", good, []string{"account-b"}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if err := verifyWebsocketExpectation(expected, mutation.value, mutation.owners); err == nil {
				t.Fatal("material terminal status or retry drift was accepted")
			}
		})
	}
}

// Numeric-status precedence has two independent edge classes: invalid
// primary values must NOT truncate the stream, while a valid high u16 must
// end it. Wrong selection on either side can replay or swallow a model turn.
func TestProdex04380WebSocketStatusU16AndFractionOracleRejectsMutations(t *testing.T) {
	cases := []struct {
		mode   string
		frames []string
		want   []string
	}{
		{mode: "ws-status-fraction", frames: []string{
			`{"type":"error","status":400.5,"status_code":400}`,
			`{"type":"response.completed","response":{"id":"resp-status-fraction"}}`,
		}, want: []string{"error", "response.completed"}},
		{mode: "ws-status-overflow", frames: []string{
			`{"type":"error","status":65536,"status_code":400}`,
			`{"type":"response.completed","response":{"id":"resp-status-overflow"}}`,
		}, want: []string{"error", "response.completed"}},
		{mode: "ws-status-high", frames: []string{`{"type":"error","status":65535}`}, want: []string{"error"}},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			expected := websocketExpectation{mode: tc.mode, want: tc.want, attempts: 1, owner: "account-a", rawFrames: tc.frames}
			good := wsProbe{types: tc.want, body: tc.frames}
			if err := verifyWebsocketExpectation(expected, good, []string{"account-a"}); err != nil {
				t.Fatal(err)
			}
			tooMany := wsProbe{types: append(append([]string(nil), tc.want...), "response.completed"), body: append(append([]string(nil), tc.frames...), `{"type":"response.completed"}`)}
			if err := verifyWebsocketExpectation(expected, tooMany, []string{"account-a"}); err == nil {
				t.Fatal("followup completion slipped past terminal oracle")
			}
			badRaw := wsProbe{types: append([]string(nil), tc.want...), body: append([]string(nil), tc.frames...)}
			badRaw.body[0] = `{"type":"error","status":400,"status_code":400}`
			if err := verifyWebsocketExpectation(expected, badRaw, []string{"account-a"}); err == nil {
				t.Fatal("changed status threshold evaded oracle")
			}
			if err := verifyWebsocketExpectation(expected, good, []string{"account-a", "account-a"}); err == nil {
				t.Fatal("retry count mutation evaded oracle")
			}
		})
	}
}
