package main

import (
	"net/http"
	"strings"
	"testing"
)

// The differential checks actual account dispatch, not simply whether the
// synthetic quota body was recognized as a 429 by both HTTP transports.
func TestProdex04383HTTP429PolicyOracleRejectsMutations(t *testing.T) {
	for _, shape := range http429PolicyShapes04383 {
		t.Run(shape, func(t *testing.T) {
			attempts := []transportAttempt04383{
				{owner: "account-a", ready: true, body: http429PolicyRequest04383},
				{owner: "account-b", ready: true, body: http429PolicyRequest04383},
			}
			reply := exchange{status: http.StatusOK, body: http429PolicyOK04383}
			if err := verifyHTTP429Policy04383(shape, reply, attempts, false); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*exchange, *[]transportAttempt04383, *bool)
			}{
				{"wrong first owner", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[0].owner = "account-b" }},
				{"wrong fallback owner", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[1].owner = "account-a" }},
				{"missing Ready", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[0].ready = false }},
				{"changed original body", func(e *exchange, a *[]transportAttempt04383, b *bool) {
					(*a)[0].body = strings.Replace((*a)[0].body, "429 code policy probe", "changed", 1)
				}},
				{"extra turn header", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[1].turn = "foreign" }},
				{"missing model attempt", func(e *exchange, a *[]transportAttempt04383, b *bool) { *a = (*a)[:1] }},
				{"duplicated model attempt", func(e *exchange, a *[]transportAttempt04383, b *bool) { *a = append(*a, (*a)[0]) }},
				{"HTTP 429 terminal", func(e *exchange, a *[]transportAttempt04383, b *bool) { e.status = http.StatusTooManyRequests }},
				{"forged response", func(e *exchange, a *[]transportAttempt04383, b *bool) { e.body = `{"id":"forged"}` }},
				{"bad auth", func(e *exchange, a *[]transportAttempt04383, b *bool) { *b = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := reply
					a := append([]transportAttempt04383(nil), attempts...)
					bad := false
					tc.mutate(&r, &a, &bad)
					if err := verifyHTTP429Policy04383(shape, r, a, bad); err == nil {
						t.Fatalf("HTTP429 oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
