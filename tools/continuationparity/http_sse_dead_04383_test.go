package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPSSEDeadChainOracleRejectsMutations(t *testing.T) {
	base := exchange{status: http.StatusOK, body: httpSSEDeadWire04383}
	attempt := transportAttempt04383{owner: "account-a", ready: true, body: httpSSEDeadRequest04383}
	if err := verifyHTTPSSEDead04383(base, []transportAttempt04383{attempt}, false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*exchange, *[]transportAttempt04383, *bool)
	}{
		{"wrong owner", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[0].owner = "account-b" }},
		{"no Ready", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[0].ready = false }},
		{"modified request", func(e *exchange, a *[]transportAttempt04383, b *bool) {
			(*a)[0].body = strings.Replace((*a)[0].body, "committed previous chain fails", "forged", 1)
		}},
		{"missing error event", func(e *exchange, a *[]transportAttempt04383, b *bool) { e.body = httpSSEDeadPrelude04383 }},
		{"rewritten error", func(e *exchange, a *[]transportAttempt04383, b *bool) {
			e.body = strings.Replace(e.body, "previous_response_not_found", "other", 1)
		}},
		{"duplicate inference", func(e *exchange, a *[]transportAttempt04383, b *bool) { *a = append(*a, (*a)[0]) }},
		{"missing inference", func(e *exchange, a *[]transportAttempt04383, b *bool) { *a = nil }},
		{"invalid auth", func(e *exchange, a *[]transportAttempt04383, b *bool) { *b = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			a := []transportAttempt04383{attempt}
			bad := false
			tc.mutate(&e, &a, &bad)
			if err := verifyHTTPSSEDead04383(e, a, bad); err == nil {
				t.Fatalf("SSE dead-chain oracle missed %s", tc.name)
			}
		})
	}
}

// Plain data-field text is not a serde_json::Value. The tagged SSE tap must
// keep both the previously verified response and committed output ID alive.
func TestProdex04383HTTPSSEPlainTextDeadMarkerOracleRejectsMutations(t *testing.T) {
	base := exchange{status: http.StatusOK, body: httpSSEDeadWireFor04383("plain-text")}
	attempt := transportAttempt04383{owner: "account-a", ready: true, body: httpSSEDeadRequest04383}
	if err := verifyHTTPSSEDeadFor04383("plain-text", base, []transportAttempt04383{attempt}, false); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*exchange, *[]transportAttempt04383, *bool)
	}{
		{"wrong owner", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[0].owner = "account-b" }},
		{"lost readiness", func(e *exchange, a *[]transportAttempt04383, b *bool) { (*a)[0].ready = false }},
		{"changed text marker", func(e *exchange, a *[]transportAttempt04383, b *bool) {
			e.body = strings.Replace(e.body, "previous_response_not_found", "other", 1)
		}},
		{"unwanted retry", func(e *exchange, a *[]transportAttempt04383, b *bool) { *a = append(*a, (*a)[0]) }},
		{"missing output", func(e *exchange, a *[]transportAttempt04383, b *bool) {
			e.body = httpSSEDeadSuffixFor04383("plain-text")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			a := []transportAttempt04383{attempt}
			bad := false
			tc.mutate(&e, &a, &bad)
			if err := verifyHTTPSSEDeadFor04383("plain-text", e, a, bad); err == nil {
				t.Fatalf("plain SSE marker oracle missed %s", tc.name)
			}
		})
	}
}
