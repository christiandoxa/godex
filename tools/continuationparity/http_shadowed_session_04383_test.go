package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPDirectShadowOracleRejectsMutations(t *testing.T) {
	for _, shape := range []string{"null", "blank", "missing", "conversation", "thread", "both", "direct", "metadata-nested", "metadata-direct", "metadata-null", "metadata-blank", "metadata-fallback", "invalid-utf8"} {
		t.Run(shape, func(t *testing.T) {
			owner := "account-a"
			if httpShadowedSessionExpectSecond04383(shape) {
				owner = "account-b"
			}
			base := httpSoftConflictResult04383{reply: exchange{status: http.StatusOK, body: httpShadowedSessionReply04383},
				attempts: []transportAttempt04383{{owner: owner, ready: true, body: httpShadowedSessionBody04383(shape), metadata: httpShadowedSessionHeaderMetadata04383(shape)}}}
			if err := verifyHTTPShadowedSession04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpSoftConflictResult04383)
			}{
				{"foreign owner", func(r *httpSoftConflictResult04383) {
					if owner == "account-a" {
						r.attempts[0].owner = "account-b"
					} else {
						r.attempts[0].owner = "account-a"
					}
				}},
				{"unready owner", func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false }},
				{"unexpected turn header", func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "untrusted" }},
				{"changed turn metadata header", func(r *httpSoftConflictResult04383) { r.attempts[0].metadata += "tampered" }},
				{"damaged opaque body", func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "independent complete request", "changed request", 1)
				}},
				{"absent upstream", func(r *httpSoftConflictResult04383) { r.attempts = nil }},
				{"duplicate model inference", func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) }},
				{"invalid synthetic auth", func(r *httpSoftConflictResult04383) { r.invalid = true }},
				{"HTTP 409", func(r *httpSoftConflictResult04383) { r.reply.status = 409 }},
				{"changed response", func(r *httpSoftConflictResult04383) { r.reply.body = `{"id":"forged"}` }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					tc.mutate(&r)
					if err := verifyHTTPShadowedSession04383(shape, r); err == nil {
						t.Fatalf("HTTP shadow oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
