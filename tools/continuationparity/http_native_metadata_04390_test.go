package main

import (
	"strings"
	"testing"
)

func TestProdex04390NativeMetadataOracleRejectsMutations(t *testing.T) {
	for _, shape := range []string{"omitted", "custom", "rotation"} {
		t.Run(shape, func(t *testing.T) {
			valid := func() httpSoftConflictResult04383 {
				attempts := []transportAttempt04383{{owner: "account-a", ready: true, body: nativeMetadataRequest04390(shape), metadata: nativeTurnMetadata04390, session: nativeSession04390}}
				if shape == "rotation" {
					second := attempts[0]
					second.owner = "account-b"
					attempts = append(attempts, second)
				}
				return httpSoftConflictResult04383{attempts: attempts, reply: exchange{status: 200, body: nativeMetadataReply04390}}
			}
			if err := verifyNativeMetadata04390(shape, valid()); err != nil {
				t.Fatal(err)
			}
			mutations := []func(*httpSoftConflictResult04383){
				func(r *httpSoftConflictResult04383) { r.invalid = true },
				func(r *httpSoftConflictResult04383) { r.attempts = r.attempts[:0] },
				func(r *httpSoftConflictResult04383) { r.attempts = append(r.attempts, r.attempts[0]) },
				func(r *httpSoftConflictResult04383) { r.reply.status = 502 },
				func(r *httpSoftConflictResult04383) { r.reply.body += "changed" },
				func(r *httpSoftConflictResult04383) { r.attempts[0].ready = false },
				func(r *httpSoftConflictResult04383) { r.attempts[0].owner = "account-b" },
				func(r *httpSoftConflictResult04383) { r.attempts[0].metadata = "{}" },
				func(r *httpSoftConflictResult04383) { r.attempts[0].session = "custom-cache-key-not-session-0164" },
				func(r *httpSoftConflictResult04383) { r.attempts[0].turn = "fabricated" },
				func(r *httpSoftConflictResult04383) { r.attempts[0].userAgent = "fabricated" },
				func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "9007199254740993", "9007199254740992", 1)
				},
				func(r *httpSoftConflictResult04383) {
					r.attempts[0].body = strings.Replace(r.attempts[0].body, "future_client_field", "dropped_field", 1)
				},
				func(r *httpSoftConflictResult04383) {
					if shape != "custom" {
						r.attempts[0].body = strings.TrimSuffix(r.attempts[0].body, "}") + `,"prompt_cache_key":"fabricated"}`
					} else {
						r.attempts[0].body = strings.Replace(r.attempts[0].body, "custom-cache-key-not-session-0164", nativeSession04390, 1)
					}
				},
			}
			for index, mutate := range mutations {
				result := valid()
				mutate(&result)
				if err := verifyNativeMetadata04390(shape, result); err == nil {
					t.Fatalf("accepted mutation %d", index)
				}
			}
		})
	}
}
