package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPSoftSessionConflictOracleRejectsMutations(t *testing.T) {
	base := httpSoftConflictResult04383{
		reply:    exchange{status: http.StatusOK, body: httpSoftConflictResponse04383},
		attempts: []transportAttempt04383{{owner: "account-a", ready: true, body: httpSoftConflictRequest04383}},
	}
	if err := verifyHTTPSoftConflict04383(base); err != nil {
		t.Fatal(err)
	}
	other := base
	other.attempts = []transportAttempt04383{{owner: "account-b", ready: true, body: httpSoftConflictRequest04383}}
	if err := verifyHTTPSoftConflict04383(other); err != nil {
		t.Fatalf("second Ready profile must remain eligible: %v", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*httpSoftConflictResult04383)
	}{
		{"HTTP local rejection", func(r *httpSoftConflictResult04383) { r.reply.status = 409 }},
		{"unexpected upstream body", func(r *httpSoftConflictResult04383) { r.reply.body = `{"error":"blocked"}` }},
		{"missing upstream request", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
		{"duplicate inference", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
		{"unverified quota", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
		{"unregistered account", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-c" }},
		{"stale turn", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "foreign-turn" }},
		{"lost opaque input", func(r *httpSoftConflictResult04383) {
			r.attempts[0].body = strings.Replace(httpSoftConflictRequest04383, `"content":"independent complete request"`, `"content":"lost"`, 1)
		}},
		{"unverified synthetic auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.attempts = append([]transportAttempt04383(nil), base.attempts...)
			tc.change(&r)
			if err := verifyHTTPSoftConflict04383(r); err == nil {
				t.Fatalf("HTTP soft session differential missed %s", tc.name)
			}
		})
	}
}
