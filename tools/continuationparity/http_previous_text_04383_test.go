package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPPreviousTextOracleRejectsMutations(t *testing.T) {
	for _, shape := range httpPreviousTextShapes04383() {
		t.Run(shape, func(t *testing.T) {
			status, body := http.StatusBadRequest, httpPreviousTextBody04383(shape)
			attempts := []transportAttempt04383{{owner: "account-a", ready: true, body: httpPreviousTextRequest04383}}
			if httpPreviousTextRecognized04383(shape) {
				status, body = http.StatusConflict, httpPreviousTextStale04383
			}
			base := httpSoftConflictResult04383{reply: exchange{status: status, body: body}, attempts: attempts}
			if err := verifyHTTPPreviousText04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				change func(*httpSoftConflictResult04383)
			}{
				{"owner diversion", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
				{"lost readiness", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"rewritten previous request", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "continue plain previous", "tampered", 1)
				}},
				{"injected turn state", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "forged" }},
				{"missing attempt", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"duplicated attempt", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"changed response", func(r *httpSoftConflictResult04383) { r.reply.body += "changed" }},
				{"HTTP 503", func(r *httpSoftConflictResult04383) { r.reply.status = 503 }},
				{"unauthenticated", func(r *httpSoftConflictResult04383) { r.invalid = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.change(&r)
					if err := verifyHTTPPreviousText04383(shape, r); err == nil {
						t.Fatalf("HTTP previous-text oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
