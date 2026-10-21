package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProdex04384DeepSeekStreamErrorOracleRejectsFallbackMutations(t *testing.T) {
	for _, name := range []string{"deepseek-sse-error-presence", "deepseek-sse-error-i64", "deepseek-sse-error-float"} {
		t.Run(name, func(t *testing.T) {
			_, code, message, _ := deepSeekStreamErrorFixture04384(name)
			good := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":%q,\"message\":%q},\"id\":\"resp_deepseek_01900000-0000-7000-8000-000000000001\"},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", time.Now().Unix(), code, message)
			if !validFixtureStreamErrorPresence04384(name, good) || !equivalentFixtureStreamErrorPresence04384(good, good) {
				t.Fatal("valid embedded-error fixture rejected")
			}
			for _, changed := range []string{
				strings.Replace(good, `"code":"`+code+`"`, `"code":"SECONDARY"`, 1),
				strings.Replace(good, `"message":"`+message+`"`, `"message":"must-not-borrow"`, 1),
				strings.Replace(good, `"sequence_number":0`, `"sequence_number":1`, 1),
				strings.Replace(good, "event: response.failed", "event: response.completed", 1),
				good + good,
			} {
				if changed == good || validFixtureStreamErrorPresence04384(name, changed) || equivalentFixtureStreamErrorPresence04384(good, changed) {
					t.Fatal("embedded-error oracle accepted a mutated event")
				}
			}
		})
	}
}
