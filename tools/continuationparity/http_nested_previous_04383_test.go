package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPNestedPreviousOracleRejectsMutations(t *testing.T) {
	for _, shape := range []string{"missing", "null", "blank", "direct"} {
		t.Run(shape, func(t *testing.T) {
			owner := "account-a"
			if shape == "direct" {
				owner = "account-b"
			}
			base := httpSoftConflictResult04383{
				reply:    exchange{status: http.StatusOK, body: httpNestedPreviousReply04383},
				attempts: []transportAttempt04383{{owner: owner, ready: true, body: httpNestedPreviousBody04383(shape)}},
			}
			if err := verifyHTTPNestedPrevious04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"cross-account owner", func(r *httpSoftConflictResult04383) {
					if owner == "account-a" {
						r.attempts[0].owner = "account-b"
					} else {
						r.attempts[0].owner = "account-a"
					}
				}},
				{"missing Ready receipt", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"stale turn", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "forged" }},
				{"missing upstream", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"duplicate upstream", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"changed nested previous", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, `"response":{"previous_response_id":"resp-nested"}`, `"response":{"previous_response_id":"changed"}`, 1)
				}},
				{"changed user history", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "independent nested previous", "changed history", 1)
				}},
				{"HTTP reject", func(r *httpSoftConflictResult04383) { r.reply.status = 409 }},
				{"forged response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"wrong"}` }},
				{"failed auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&r)
					if err := verifyHTTPNestedPrevious04383(shape, r); err == nil {
						t.Fatalf("nested previous oracle missed mutation: %s", tc.name)
					}
				})
			}
		})
	}
}
