package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04383HTTPPromptCacheOracleRejectsMutations(t *testing.T) {
	for _, shape := range httpPromptCacheShapes04383 {
		t.Run(shape, func(t *testing.T) {
			base := httpPromptCacheResult04383{}
			for _, stage := range []string{"seed-main", "seed-second", "restore-main", "fresh"} {
				base.attempts = append(base.attempts, transportAttempt04383{
					owner: httpPromptCacheWantOwner04383(shape, stage),
					ready: true, body: httpPromptCacheRequest04383(stage),
				})
				base.replies = append(base.replies, exchange{status: http.StatusOK, body: httpPromptCacheReply04383(shape, stage)})
			}
			if err := verifyHTTPPromptCache04383(shape, base); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name   string
				mutate func(*httpPromptCacheResult04383)
			}{
				{"wrong cache owner", func(r *httpPromptCacheResult04383) {
					if r.attempts[3].owner == "account-a" {
						r.attempts[3].owner = "account-b"
					} else {
						r.attempts[3].owner = "account-a"
					}
				}},
				{"wrong seed owner", func(r *httpPromptCacheResult04383) { r.attempts[1].owner = "account-a" }},
				{"missing quota evidence", func(r *httpPromptCacheResult04383) { r.attempts[1].ready = false }},
				{"damaged cache key", func(r *httpPromptCacheResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "cache-affinity-synthetic", "changed", 1)
				}},
				{"injected turn", func(r *httpPromptCacheResult04383) { r.attempts[2].turn = "foreign" }},
				{"injected session metadata", func(r *httpPromptCacheResult04383) { r.attempts[1].metadata = "foreign" }},
				{"injected thread", func(r *httpPromptCacheResult04383) { r.attempts[2].trace = "foreign" }},
				{"extra inference", func(r *httpPromptCacheResult04383) { r.attempts = append(r.attempts, r.attempts[3]) }},
				{"missing inference", func(r *httpPromptCacheResult04383) { r.attempts = r.attempts[:3] }},
				{"invalid credentials", func(r *httpPromptCacheResult04383) { r.invalid = true }},
				{"HTTP 429", func(r *httpPromptCacheResult04383) { r.replies[3].status = http.StatusTooManyRequests }},
				{"forged response", func(r *httpPromptCacheResult04383) { r.replies[0].body = `{"id":"forged"}` }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					r := base
					r.attempts = append([]transportAttempt04383(nil), base.attempts...)
					r.replies = append([]exchange(nil), base.replies...)
					tc.mutate(&r)
					if err := verifyHTTPPromptCache04383(shape, r); err == nil {
						t.Fatalf("prompt cache oracle missed %s", tc.name)
					}
				})
			}
		})
	}
}
