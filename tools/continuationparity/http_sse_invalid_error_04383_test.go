package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPSSEInvalidErrorOracleRejectsMutations(t *testing.T) {
	for _, shape := range httpSSEInvalidErrorShapes04383() {
		t.Run(shape, func(t *testing.T) {
			base := httpSoftConflictResult04383{reply: exchange{status: http.StatusOK, body: httpSSEInvalidErrorBody04383(shape)},
				attempts: []transportAttempt04383{{owner: "account-a", ready: true, body: httpSSEInvalidErrorRequest04383}}}
			if err := verifyHTTPSSEInvalidError04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"wrong owner", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
				{"no Ready", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"changed input", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "independent invalid SSE", "changed", 1)
				}},
				{"changed SSE", func(r *httpSoftConflictResult04383) { r.reply.body += "forged" }},
				{"not HTTP 200", func(r *httpSoftConflictResult04383) { r.reply.status = 503 }},
				{"missing model", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"replayed model", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"injected turn", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "foreign" }},
				{"bad authentication", func(r *httpSoftConflictResult04383) { r.invalid = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					mutated := base
					mutated.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&mutated)
					if err := verifyHTTPSSEInvalidError04383(shape, mutated); err == nil {
						t.Fatalf("malformed SSE error oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
