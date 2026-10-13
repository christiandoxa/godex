package routing

import (
	"net/http"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04379KeepaliveDoesNotCommitSSEBeforeOverload(t *testing.T) {
	const wire = "data: {\"type\":\"keepalive\"}\n\n" +
		"data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"server_is_overloaded\"}}}\n\n"
	for split := 0; split <= len(wire); split++ {
		decoder := sse.NewDecoder(4096)
		headers := make(http.Header)
		now := time.Unix(0, 0)
		committed := false
		for _, chunk := range []string{wire[:split], wire[split:]} {
			result, done := startupStreamOutcome(decoder, []byte(chunk), headers, now, "openai")
			if !done {
				continue
			}
			if result.kind != responseRetry || !result.transient || !result.firstEventRetry {
				t.Fatalf("split %d committed before failure: %#v", split, result)
			}
			committed = true
		}
		if !committed {
			t.Fatalf("split %d never recognized terminal capacity failure", split)
		}
	}

	for _, event := range []string{"response.output_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta"} {
		prefix := "data: {\"type\":\"keepalive\"}\n\n" +
			"data: {\"type\":\"" + event + "\",\"delta\":\"visible\"}\n\n"
		decoder := sse.NewDecoder(4096)
		outcome, done := startupStreamOutcome(decoder, []byte(prefix), make(http.Header), time.Unix(0, 0), "openai")
		if !done || outcome.kind != responsePass {
			t.Fatalf("visible event %s not committed: outcome=%#v done=%t", event, outcome, done)
		}
	}
}
