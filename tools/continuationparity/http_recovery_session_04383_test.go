package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPRecoverySessionOracleRejectsMutations(t *testing.T) {
	for _, shape := range []string{"null", "blank", "nested", "direct", "metadata-null", "metadata-nested", "header-override",
		"invalid-utf8", "invalid-surrogate", "invalid-number", "invalid-depth", "valid-unicode", "valid-finite", "metadata-invalid", "metadata-valid",
		"response-invalid-surrogate", "response-invalid-utf8", "response-invalid-depth",
		"response-type-space", "response-type-upper", "response-type-unicode", "response-code-space",
		"response-code-upper", "response-code-exact", "response-code-unicode", "response-tool-param-upper",
		"response-message-unicode-space", "response-node-budget"} {
		t.Run(shape, func(t *testing.T) {
			status, response := http.StatusBadRequest, httpRecoverySessionErrorForShape04383(shape)
			if httpRecoverySessionExpectRetry04383(shape) {
				status, response = http.StatusOK, httpRecoverySessionResponse04383
			} else if shape == "response-code-exact" {
				status, response = http.StatusConflict, httpPreviousTextStale04383
			}
			base := httpSoftConflictResult04383{reply: exchange{status: status, body: response},
				attempts: []transportAttempt04383{{owner: "account-a", ready: true,
					body: httpRecoverySessionRequest04383(shape), metadata: httpRecoverySessionMetadata04383(shape)}},
			}
			if httpRecoverySessionExpectRetry04383(shape) {
				base.attempts = append(base.attempts, transportAttempt04383{owner: "account-a", ready: true,
					body: httpRecoverySessionReplay04383(shape), metadata: httpRecoverySessionMetadata04383(shape)})
			}
			if err := verifyHTTPRecoverySession04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"wrong owner", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
				{"unready", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"changed opaque request", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "continue", "changed", 1)
				}},
				{"forged metadata", func(r *httpSoftConflictResult04383) { r.attempts[0].metadata += "foreign" }},
				{"missing attempt", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"extra attempt", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"changed response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"forged"}` }},
				{"changed status", func(r *httpSoftConflictResult04383) { r.reply.status = 503 }},
				{"invalid auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&r)
					if err := verifyHTTPRecoverySession04383(shape, r); err == nil {
						t.Fatalf("HTTP recovery session oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
