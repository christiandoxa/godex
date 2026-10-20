package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPUnsupportedThreadOracleRejectsMutations(t *testing.T) {
	base := httpSoftConflictResult04383{reply: exchange{status: http.StatusOK, body: httpUnsupportedThreadReply04383("resp-thread-independent")}}
	for _, step := range httpUnsupportedThreadSteps04383 {
		base.attempts = append(base.attempts, transportAttempt04383{
			owner: step.owner, body: step.body, trace: step.trace, ready: true,
		})
	}
	if err := verifyHTTPUnsupportedThread04383(base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*httpSoftConflictResult04383)
	}{
		{"foreign owner", func(r *httpSoftConflictResult04383) { r.attempts[2].owner = "account-b" }},
		{"lost trace header", func(r *httpSoftConflictResult04383) { r.attempts[2].trace = "" }},
		{"added trace header", func(r *httpSoftConflictResult04383) { r.attempts[1].trace = "trace-only" }},
		{"missing Ready evidence", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
		{"changed full request", func(r *httpSoftConflictResult04383) {
			r.attempts[2].body = strings.Replace(r.attempts[2].body, "thread independent", "changed", 1)
		}},
		{"duplicated inference", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[2]) }},
		{"missing inference", func(r *httpSoftConflictResult04383) { r.attempts = r.attempts[:2] }},
		{"leaked turn token", func(r *httpSoftConflictResult04383) { r.attempts[1].turn = "foreign" }},
		{"changed turn metadata", func(r *httpSoftConflictResult04383) { r.attempts[1].metadata = `{"session_id":"foreign"}` }},
		{"invalid auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
		{"HTTP 503", func(r *httpSoftConflictResult04383) { r.reply.status = 503 }},
		{"changed reply", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"wrong"}` }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.attempts = append([]transportAttempt04383(nil), base.attempts...)
			tc.mutate(&r)
			if err := verifyHTTPUnsupportedThread04383(r); err == nil {
				t.Fatalf("unsupported thread oracle missed %s", tc.name)
			}
		})
	}
}
