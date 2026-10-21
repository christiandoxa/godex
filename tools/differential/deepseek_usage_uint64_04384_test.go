package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekUnsignedUsageOracleChecksExactIntegerTokens(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name, body := "deepseek-usage-u64", strings.Replace(syntheticFixtureResponse, `{`, `{"created_at":18446744073709551615,`, 1)
		if stream {
			name, body = "deepseek-sse-usage-u64", fixtureStreamWire("msg_deepseek_42")
			body = strings.ReplaceAll(body, `"created_at":1`, `"created_at":18446744073709551615`)
		}
		body = strings.Replace(body, `{"input_tokens":2,"output_tokens":1,"total_tokens":3}`, deepSeekUnsignedUsageOutput04384, 1)
		t.Run(name, func(t *testing.T) {
			if !validDeepSeekUnsignedUsage04384(body, stream) || stream && !equivalentDeepSeekUnsignedSSE04384(body, body) {
				t.Fatal("exact unsigned fixture rejected")
			}
			for _, mutation := range []struct{ old, replacement string }{
				{"9007199254740993", "9007199254740992"},
				{"18446744073709551615", "18446744073709551616"},
				{"9223372036854775808", "9223372036854775807"},
				{`"reasoning_tokens":18446744073709551615`, `"reasoning_tokens":0`},
				{`"total_tokens":18446744073709551615`, `"total_tokens":9007199254740992`},
				{`"cached_tokens":9223372036854775808`, `"cached_tokens":null`},
				{"synthetic-ok", "wrong-output"},
			} {
				changed := strings.Replace(body, mutation.old, mutation.replacement, 1)
				if changed == body || validDeepSeekUnsignedUsage04384(changed, stream) || stream && equivalentDeepSeekUnsignedSSE04384(body, changed) {
					t.Fatalf("unsigned oracle accepted mutation %q", mutation.old)
				}
			}
			if validDeepSeekUnsignedUsage04384(body+"{}", stream) {
				t.Fatal("oracle accepted trailing JSON")
			}
			request := syntheticFixtureRequest
			if stream {
				request = strings.Replace(request, `"stream":false`, `"stream":true`, 1)
			}
			run := productRun{Name: "prodex", Client: exchange{Status: http.StatusOK, Body: body}, Upstream: []upstreamRequest{{Method: "POST", Path: "/v1/chat/completions", AuthOK: true, KeySlot: "single", Body: request}}}
			scenario := scenarioResult{Name: name, Runs: []productRun{run, run}}
			if failures := scenarioInvariants(scenario); len(failures) != 0 {
				t.Fatalf("valid unsigned scenario rejected: %v", failures)
			}
			scenario.Runs[1].Client.Body = strings.Replace(body, "9007199254740993", "9007199254740992", 1)
			if failures := scenarioInvariants(scenario); len(failures) == 0 {
				t.Fatal("rounded usage escaped scenario assertions")
			}
		})
	}
}
