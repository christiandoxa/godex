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
	if err := verifyHTTPPreviousSession04383(base, false); err != nil {
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
			if err := verifyHTTPPreviousSession04383(r, false); err == nil {
				t.Fatalf("hard previous owner mutation escaped: %s", tc.name)
			}
		})
	}
}

// The turn-only successor is independently pinned: unlike a previous-ID
// request it must forward both the original turn token and the complete body
// on its only attempt, while preserving the hard owner's session outcome.
func TestProdex04383HTTPTurnSessionOracleRejectsMutations(t *testing.T) {
	base := httpSoftConflictResult04383{
		reply:    exchange{status: http.StatusOK, body: httpPreviousSessionResponse04383},
		attempts: []transportAttempt04383{{owner: "account-a", ready: true, turn: "turn-a", body: httpTurnSessionRequest04383}},
	}
	if err := verifyHTTPPreviousSession04383(base, true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*httpSoftConflictResult04383)
	}{
		{"selected foreign owner", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
		{"missing turn header", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "" }},
		{"stale turn header", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "turn-wrong" }},
		{"removed turn body", func(r *httpSoftConflictResult04383) {
			r.attempts[0].body = strings.Replace(httpTurnSessionRequest04383, `"x-codex-turn-state":"turn-a",`, "", 1)
		}},
		{"removed session", func(r *httpSoftConflictResult04383) {
			r.attempts[0].body = strings.Replace(httpTurnSessionRequest04383, `"session_id":"http-hard-session",`, "", 1)
		}},
		{"duplicate provider request", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
		{"unready profile", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
		{"altered response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"wrong"}` }},
		{"invalid auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.attempts = append([]transportAttempt04383(nil), base.attempts...)
			tc.mutate(&r)
			if err := verifyHTTPPreviousSession04383(r, true); err == nil {
				t.Fatalf("turn session mutation escaped: %s", tc.name)
			}
		})
	}
}
