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
