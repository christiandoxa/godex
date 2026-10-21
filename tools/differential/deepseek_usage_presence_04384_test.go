package main

import (
	"strings"
	"testing"
)

func TestProdex04384DeepSeekNullUsageOracleRejectsMissingAndStaleCounters(t *testing.T) {
	for _, stream := range []bool{false, true} {
		body := syntheticFixtureResponse
		if stream {
			body = fixtureStreamWire("msg_deepseek_42")
		}
		const ordinary = `"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}`
		const zero = `"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}`
		body = strings.Replace(body, ordinary, zero, 1)
		if !validDeepSeekNullUsage04384(body, stream) || stream && !equivalentDeepSeekNullUsageSSE04384(body, body) {
			t.Fatal("present-null usage fixture rejected")
		}
		for _, changed := range []string{
			strings.Replace(body, zero, ordinary, 1),
			strings.Replace(body, `,"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, "", 1),
			strings.Replace(body, `"input_tokens":0`, `"input_tokens":0.0`, 1),
			strings.Replace(body, `"total_tokens":0`, `"total_tokens":3`, 1),
			strings.Replace(body, zero, `"usage":null`, 1),
			strings.Replace(body, "synthetic-ok", "wrong-output", 1),
			body + "{}",
		} {
			if changed == body || validDeepSeekNullUsage04384(changed, stream) || stream && equivalentDeepSeekNullUsageSSE04384(body, changed) {
				t.Fatal("present-null oracle accepted missing, stale or malformed usage")
			}
		}
	}
}
