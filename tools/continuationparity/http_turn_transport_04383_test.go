package main

import (
	"strings"
	"testing"
)

func TestProdex04383HTTPResponsesTurnOracleRejectsMutations(t *testing.T) {
	for _, scenario := range []string{"turn-only", "with-session", "previous-response"} {
		t.Run(scenario, func(t *testing.T) {
			valid := func() httpTransportResult04383 {
				v := httpTransportResult04383{
					reply:    exchange{status: 503, body: httpTurnUnavailable04383},
					attempts: []transportAttempt04383{{owner: "account-a", turn: "turn-a", body: httpTransportBody04383(scenario), ready: true}},
					owners:   map[string]string{"turn": "main", "session": "main", "unrelated": "main"},
				}
				if scenario == "previous-response" {
					v.reply = exchange{status: 400, body: compactRetry04383}
					v.owners["turn"], v.owners["unrelated"] = "", ""
				}
				return v
			}
			if err := verifyHTTPResponsesTurn04383(scenario, valid()); err != nil {
				t.Fatal(err)
			}
			for name, mutate := range map[string]func(*httpTransportResult04383){
				"auth":            func(v *httpTransportResult04383) { v.invalid = true },
				"replay":          func(v *httpTransportResult04383) { v.attempts = append(v.attempts, v.attempts[0]) },
				"missing request": func(v *httpTransportResult04383) { v.attempts = nil },
				"readiness":       func(v *httpTransportResult04383) { v.attempts[0].ready = false },
				"owner":           func(v *httpTransportResult04383) { v.attempts[0].owner = "account-b" },
				"turn header":     func(v *httpTransportResult04383) { v.attempts[0].turn = "" },
				"nested turn": func(v *httpTransportResult04383) {
					v.attempts[0].body = strings.ReplaceAll(v.attempts[0].body, "turn-a", "stale")
				},
				"large number": func(v *httpTransportResult04383) {
					v.attempts[0].body = strings.ReplaceAll(v.attempts[0].body, "9007199254740993", "9007199254740992")
				},
				"status":             func(v *httpTransportResult04383) { v.reply.status = 200 },
				"output":             func(v *httpTransportResult04383) { v.reply.body = "" },
				"turn binding":       func(v *httpTransportResult04383) { v.owners["turn"] = "second" },
				"response binding":   func(v *httpTransportResult04383) { v.owners["response"] = "second" },
				"session binding":    func(v *httpTransportResult04383) { v.owners["session"] = "second" },
				"unrelated response": func(v *httpTransportResult04383) { v.owners["unrelated"] = "second" },
			} {
				t.Run(name, func(t *testing.T) {
					v := valid()
					mutate(&v)
					if verifyHTTPResponsesTurn04383(scenario, v) == nil {
						t.Fatal("HTTP Responses turn mutation escaped the oracle")
					}
				})
			}
		})
	}
}
