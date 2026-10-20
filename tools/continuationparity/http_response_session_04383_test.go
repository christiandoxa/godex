package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPResponseSessionOracleRejectsMutations(t *testing.T) {
	for _, shape := range httpResponseSessionShapes04383 {
		t.Run(shape, func(t *testing.T) {
			base := httpSoftConflictResult04383{
				reply:    exchange{status: http.StatusOK, body: httpResponseSessionReply04383(shape)},
				attempts: []transportAttempt04383{{owner: "account-a", ready: true, body: httpResponseSessionRequest04383(shape)}},
			}
			if err := verifyHTTPResponseSession04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"wrong owner", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
				{"unready owner", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"different response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"forged"}` }},
				{"changed body", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, `"model":"gpt-6"`, `"model":"gpt-5.6"`, 1)
				}},
				{"new turn", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "foreign" }},
				{"injected metadata", func(r *httpSoftConflictResult04383) { r.attempts[0].metadata = `{"session_id":"foreign"}` }},
				{"injected thread", func(r *httpSoftConflictResult04383) { r.attempts[0].trace = "foreign" }},
				{"no inference", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"duplicate inference", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"invalid auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
				{"bad status", func(r *httpSoftConflictResult04383) { r.reply.status = 409 }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&r)
					if err := verifyHTTPResponseSession04383(shape, r); err == nil {
						t.Fatalf("response-only session oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
