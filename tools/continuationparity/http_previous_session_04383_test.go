package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPPreviousSessionOracleRejectsMutations(t *testing.T) {
	base := httpSoftConflictResult04383{
		reply:    exchange{status: http.StatusOK, body: httpPreviousSessionResponse04383},
		attempts: []transportAttempt04383{{owner: "account-a", ready: true, body: httpPreviousSessionRequest04383}},
	}
	if err := verifyHTTPPreviousSession04383(base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*httpSoftConflictResult04383)
	}{
		{"foreign selected", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
		{"missing Ready receipt", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
		{"missing request", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
		{"replayed previous owner", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
		{"stale turn header", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "foreign-turn" }},
		{"missing previous input", func(r *httpSoftConflictResult04383) {
			r.attempts[0].body = strings.Replace(httpPreviousSessionRequest04383, `"previous_response_id":"resp-main",`, "", 1)
		}},
		{"missing session", func(r *httpSoftConflictResult04383) {
			r.attempts[0].body = strings.Replace(httpPreviousSessionRequest04383, `"session_id":"http-hard-session",`, "", 1)
		}},
		{"early unauthorized", func(r *httpSoftConflictResult04383) { r.reply.status = http.StatusUnauthorized }},
		{"changed response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"wrong"}` }},
		{"invalid synthetic auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.attempts = append([]transportAttempt04383(nil), base.attempts...)
			tc.mutate(&r)
			if err := verifyHTTPPreviousSession04383(r); err == nil {
				t.Fatalf("hard previous owner mutation escaped: %s", tc.name)
			}
		})
	}
}
