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

// Each turn-state lifetime is checked independently: the same response ID
// may inherit an earlier stream turn or be backfilled after the first output
// chunk already committed. A 200 alone does not prove the lineage contract.
func TestProdex04383HTTPSSETurnLineageCrossChunkOracleRejectsMutations(t *testing.T) {
	for _, shape := range []string{"distinct", "inherit", "late", "large-committed", "request-turn"} {
		t.Run(shape, func(t *testing.T) {
			base := httpSoftConflictResult04383{
				reply: exchange{status: http.StatusOK, body: httpSSETurnLineageWireFor04383(shape)},
				attempts: []transportAttempt04383{{owner: "account-a", ready: true,
					body: httpSSETurnLineageRequestFor04383(shape), turn: httpSSETurnLineageHeaderFor04383(shape)}},
			}
			if err := verifyHTTPSSETurnLineageFor04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"foreign owner", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
				{"no Ready", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"second turn injected", func(r *httpSoftConflictResult04383) {
					r.reply.body = strings.Replace(r.reply.body, `"id":"resp-sse-two"`, `"id":"forged"`, 1)
				}},
				{"first chunk missing", func(r *httpSoftConflictResult04383) {
					r.reply.body = strings.Replace(r.reply.body, `"delta":"first"`, `"delta":"missing"`, 1)
				}},
				{"altered request turn", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "foreign" }},
				{"extra upstream request", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"missing inference", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"unauthenticated", func(r *httpSoftConflictResult04383) { r.invalid = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					got := base
					got.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&got)
					if err := verifyHTTPSSETurnLineageFor04383(shape, got); err == nil {
						t.Fatalf("SSE lineage %s oracle missed %s", shape, tc.name)
					}
				})
			}
		})
	}
}

// The fixture warmup request must not be mistaken for the SSE model
// inference. Authentication/readiness is independently checked in the
// executable oracle, not simulated by a local unauthenticated test.
func TestProdex04383HTTPSSETurnLineageWarmupIsAccepted(t *testing.T) {
	const scenario = "http-sse-turn-lineage-request-turn"
	warmup := []byte(`{"previous_response_id":"resp-ready-b","input":[]}`)
	if httpSSETurnLineageTestRequest04383(scenario, warmup) {
		t.Fatal("warmup incorrectly intercepted by SSE lineage guard")
	}
	if !httpSSETurnLineageTestRequest04383(scenario, []byte(httpSSETurnLineageRequestFor04383("request-turn"))) {
		t.Fatal("expected SSE test request not recognized")
	}
}
