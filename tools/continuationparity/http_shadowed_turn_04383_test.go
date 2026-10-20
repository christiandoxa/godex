package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPDirectTurnShadowOracleRejectsMutations(t *testing.T) {
	for _, shape := range []string{"null", "blank", "missing", "header"} {
		t.Run(shape, func(t *testing.T) {
			owner := "account-a"
			turn := ""
			if shape == "header" {
				owner = "account-b"
				turn = "shadow-turn"
			}
			base := httpSoftConflictResult04383{reply: exchange{status: http.StatusOK, body: httpShadowedTurnReply04383},
				attempts: []transportAttempt04383{{owner: owner, ready: true, turn: turn, body: httpShadowedTurnBody04383(shape)}}}
			if err := verifyHTTPShadowedTurn04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"wrong owner", func(r *httpSoftConflictResult04383) {
					if owner == "account-a" {
						r.attempts[0].owner = "account-b"
					} else {
						r.attempts[0].owner = "account-a"
					}
				}},
				{"unready", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"changed turn header", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "wrong-turn" }},
				{"changed body", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "independent complete request", "changed request", 1)
				}},
				{"missing upstream", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"duplicate upstream", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"unauthenticated", func(r *httpSoftConflictResult04383) { r.invalid = true }},
				{"HTTP 503", func(r *httpSoftConflictResult04383) { r.reply.status = 503 }},
				{"altered response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"forged"}` }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&r)
					if err := verifyHTTPShadowedTurn04383(shape, r); err == nil {
						t.Fatalf("HTTP turn shadow oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
