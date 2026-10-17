package main

import "testing"

func TestProdex04382WebSocketCapacityOracleRejectsMaterialMutations(t *testing.T) {
	fixture := websocketCapacityResult04382{handshakes: 5, attempts: make([]attempt, 6), completed: make([]string, 6)}
	for i := range fixture.attempts {
		fixture.attempts[i] = attempt{account: "account-a", mode: capacityMode04382, body: capacityRequestBody04382}
		fixture.completed[i] = "resp-default"
	}
	if err := verifyWebSocketCapacity04382(fixture); err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		change func(*websocketCapacityResult04382)
	}{
		{"permit never released or reconnect", func(v *websocketCapacityResult04382) { v.handshakes++ }},
		{"missed upstream turn", func(v *websocketCapacityResult04382) { v.attempts = v.attempts[:5] }},
		{"duplicate dispatch", func(v *websocketCapacityResult04382) { v.attempts = append(v.attempts, v.attempts[0]) }},
		{"owner rotated", func(v *websocketCapacityResult04382) { v.attempts[3].account = "account-b" }},
		{"request body mutated", func(v *websocketCapacityResult04382) { v.attempts[3].body = `{"type":"response.create"}` }},
		{"different mode", func(v *websocketCapacityResult04382) { v.attempts[3].mode = "other" }},
		{"lost client completion", func(v *websocketCapacityResult04382) { v.completed = v.completed[:5] }},
		{"unexpected response identity", func(v *websocketCapacityResult04382) { v.completed[5] = "resp-other" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			copyValue := websocketCapacityResult04382{
				handshakes: fixture.handshakes,
				attempts:   append([]attempt(nil), fixture.attempts...),
				completed:  append([]string(nil), fixture.completed...),
			}
			tc.change(&copyValue)
			if err := verifyWebSocketCapacity04382(copyValue); err == nil {
				t.Fatalf("0.438.2 idle-permit mutation %q escaped exact oracle", tc.name)
			}
		})
	}
}
