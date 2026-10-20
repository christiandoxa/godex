package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPProfileStatusOracleRejectsMutations(t *testing.T) {
	for _, shape := range httpProfileStatusShapes04383() {
		t.Run(shape, func(t *testing.T) {
			code, body := httpProfileStatusCode04383(shape), httpProfileStatusBody04383(shape)
			attempts := []transportAttempt04383{{owner: "account-a", ready: true, body: httpProfileStatusRequest04383}}
			if httpProfileStatusShouldRotate04383(shape) {
				code, body = http.StatusOK, httpProfileStatusSuccess04383
				attempts = append(attempts, transportAttempt04383{owner: "account-b", ready: true, body: httpProfileStatusRequest04383})
			}
			base := httpSoftConflictResult04383{reply: exchange{status: code, body: body}, attempts: attempts}
			if err := verifyHTTPProfileStatus04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"wrong owner", func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" }},
				{"lost readiness", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"injected turn", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "forged" }},
				{"rewritten original", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "profile policy independent", "changed", 1)
				}},
				{"missing inference", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"duplicated inference", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"forged response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"forged"}` }},
				{"wrong status", func(r *httpSoftConflictResult04383) {
					if r.reply.status == 200 {
						r.reply.status = 403
					} else {
						r.reply.status = 200
					}
				}},
				{"invalid auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&r)
					if err := verifyHTTPProfileStatus04383(shape, r); err == nil {
						t.Fatalf("profile status mutation undetected: %s", tc.name)
					}
				})
			}
		})
	}
}
