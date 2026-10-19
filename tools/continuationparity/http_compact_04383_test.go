package main

import (
	"strings"
	"testing"
)

func TestProdex04383HTTPCompactOracleRejectsMutations(t *testing.T) {
	for _, scenario := range []string{"turn-only", "with-session", "previous-response"} {
		t.Run(scenario, func(t *testing.T) {
			valid := func() httpTransportResult04383 {
				v := httpTransportResult04383{
					reply:  exchange{status: 200, body: compactReply04383},
					owners: map[string]string{"session": "main", "unrelated": "main"},
					attempts: []transportAttempt04383{
						{owner: "account-a", turn: "turn-a", body: httpTransportBody04383(scenario), ready: true},
						{owner: "account-b", turn: "turn-a", body: httpTransportBody04383(scenario), ready: true},
					},
				}
				if scenario == "with-session" {
					v.owners["session"] = "__prodex_hard_binding_conflict__"
				} else if scenario == "previous-response" {
					v.attempts = v.attempts[:1]
					v.reply = exchange{status: 400, body: compactRetry04383}
					v.owners["unrelated"] = ""
				}
				return v
			}
			if err := verifyHTTPCompact04383(scenario, valid()); err != nil {
				t.Fatal(err)
			}
			mutations := map[string]func(*httpTransportResult04383){
				"auth":             func(v *httpTransportResult04383) { v.invalid = true },
				"extra dispatch":   func(v *httpTransportResult04383) { v.attempts = append(v.attempts, v.attempts[0]) },
				"missing dispatch": func(v *httpTransportResult04383) { v.attempts = v.attempts[:len(v.attempts)-1] },
				"readiness":        func(v *httpTransportResult04383) { v.attempts[0].ready = false },
				"owner":            func(v *httpTransportResult04383) { v.attempts[0].owner = "account-b" },
				"turn header":      func(v *httpTransportResult04383) { v.attempts[0].turn = "" },
				"nested turn": func(v *httpTransportResult04383) {
					v.attempts[0].body = strings.ReplaceAll(v.attempts[0].body, "turn-a", "stale")
				},
				"large number": func(v *httpTransportResult04383) {
					v.attempts[0].body = strings.ReplaceAll(v.attempts[0].body, "9007199254740993", "9007199254740992")
				},
				"status":             func(v *httpTransportResult04383) { v.reply.status = 502 },
				"output":             func(v *httpTransportResult04383) { v.reply.body = "" },
				"turn rebound":       func(v *httpTransportResult04383) { v.owners["turn"] = "second" },
				"response rebound":   func(v *httpTransportResult04383) { v.owners["response"] = "second" },
				"session":            func(v *httpTransportResult04383) { v.owners["session"] = "second" },
				"unrelated response": func(v *httpTransportResult04383) { v.owners["unrelated"] = "second" },
			}
			if scenario != "previous-response" {
				mutations["alternate wire"] = func(v *httpTransportResult04383) { v.attempts[1].body = `{}` }
				mutations["alternate owner"] = func(v *httpTransportResult04383) { v.attempts[1].owner = "account-a" }
			}
			for name, mutate := range mutations {
				t.Run(name, func(t *testing.T) {
					v := valid()
					mutate(&v)
					if verifyHTTPCompact04383(scenario, v) == nil {
						t.Fatal("Compact mutation escaped the executable oracle")
					}
				})
			}
		})
	}
}
