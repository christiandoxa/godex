package main

import (
	"strings"
	"testing"
)

func TestProdex04384DeepSeekEmptyIdentityOracleRejectsFallbackAndLateReplacement(t *testing.T) {
	for _, stream := range []bool{false, true} {
		body := strings.Replace(syntheticFixtureResponse, `{`, `{"id":"",`, 1)
		if stream {
			body = fixtureStreamWire("msg_deepseek_42")
			body = strings.ReplaceAll(body, `"id":"chatcmpl-differential-stream"`, `"id":""`)
			body = strings.ReplaceAll(body, `"response_id":"chatcmpl-differential-stream"`, `"response_id":""`)
		}
		body = strings.Replace(body, `"model":"deepseek-v4-pro"`, `"model":""`, 1)
		if !validDeepSeekEmptyIdentity04384(body, stream) || stream && !equivalentDeepSeekEmptyIdentitySSE04384(body, body) {
			t.Fatal("present-empty fixture rejected")
		}
		for _, mutation := range []struct{ old, replacement string }{
			{`"id":""`, `"id":"must-not-replace"`},
			{`"model":""`, `"model":"deepseek-chat"`},
			{`"model":""`, `"model":null`},
			{`"model":""`, `"wrong":""`},
			{"synthetic-ok", "wrong-output"},
		} {
			changed := strings.Replace(body, mutation.old, mutation.replacement, 1)
			if changed == body || validDeepSeekEmptyIdentity04384(changed, stream) || stream && equivalentDeepSeekEmptyIdentitySSE04384(body, changed) {
				t.Fatal("empty-identity oracle accepted replacement or missing identity")
			}
		}
		if stream {
			changed := strings.Replace(body, `"response_id":""`, `"response_id":"must-not-replace"`, 1)
			if validDeepSeekEmptyIdentity04384(changed, true) || equivalentDeepSeekEmptyIdentitySSE04384(body, changed) {
				t.Fatal("empty-identity oracle accepted a late event identity change")
			}
		}
	}
}
