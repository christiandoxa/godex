package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProdex04384WireLimitOracleRejectsWrongErrorsAndPartialOutput(t *testing.T) {
	for _, scenario := range []string{"sse-line-limit", "sse-event-limit", "sse-comment-utf8"} {
		message := wireLimitMessage04384(scenario)
		good := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":\"provider_stream_error\",\"message\":%q},\"id\":\"resp_deepseek_01900000-0000-7000-8000-000000000001\"},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", time.Now().Unix(), message)
		if !validWireLimitSSE04384(scenario, good) || !equivalentWireLimitSSE04384(good, good) {
			t.Fatal("exact wire error fixture rejected")
		}
		for _, changed := range []string{
			strings.Replace(good, message, "wrong error", 1),
			strings.Replace(good, `"sequence_number":0`, `"sequence_number":1`, 1),
			strings.Replace(good, "provider_stream_error", "wrong_code", 1),
			good + good,
			"event: response.output_text.delta\r\ndata: {}\r\n\r\n" + good,
		} {
			if validWireLimitSSE04384(scenario, changed) || equivalentWireLimitSSE04384(good, changed) {
				t.Fatal("wire error oracle accepted a changed error or partial output")
			}
		}
	}
}
