package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPSSETurnLineageOracleRejectsMutations(t *testing.T) {
	base := httpSoftConflictResult04383{
		reply:    exchange{status: http.StatusOK, body: httpSSETurnLineageWire04383},
		attempts: []transportAttempt04383{{owner: "account-a", ready: true, body: httpSSETurnLineageRequest04383}},
	}
	if err := verifyHTTPSSETurnLineage04383(base); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*httpSoftConflictResult04383)
	}{
		{"wrong owner", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
		{"missing ready", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
		{"rewritten request", func(r *httpSoftConflictResult04383) {
			r.attempts[0].body = strings.Replace(r.attempts[0].body, "separate turn states", "forged", 1)
		}},
		{"rewritten first token", func(r *httpSoftConflictResult04383) {
			r.reply.body = strings.Replace(r.reply.body, "state-one", "state-two", 1)
		}},
		{"dropped second event", func(r *httpSoftConflictResult04383) { r.reply.body = strings.Split(r.reply.body, "data: ")[1] }},
		{"duplicate inference", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
		{"missing inference", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
		{"leaked turn header", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "state-one" }},
		{"invalid auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
		{"HTTP reject", func(r *httpSoftConflictResult04383) { r.reply.status = 503 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.attempts = append([]transportAttempt04383(nil), base.attempts...)
			tc.mutate(&r)
			if err := verifyHTTPSSETurnLineage04383(r); err == nil {
				t.Fatalf("SSE oracle missed %s", tc.name)
			}
		})
	}
}
