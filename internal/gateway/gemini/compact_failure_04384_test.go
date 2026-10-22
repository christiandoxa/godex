package gemini

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestProdex04384GeminiCompactFailureReasonPriority(t *testing.T) {
	for _, fixture := range []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"},
		{&net.OpError{Op: "dial", Err: errors.New("synthetic connect failure")}, "unavailable"},
		{&net.OpError{Op: "read", Err: errors.New("synthetic read failure")}, "provider-error"},
		{errors.New("UNAVAILABLE timeout"), "timeout"},
		{errors.New("connection refused: unsupported"), "unavailable"},
		{errors.New("unsupported invalid format"), "unsupported"},
		{errors.New("failed to parse response"), "invalid-response"},
		{errors.New("unexpectedly returned stream"), "invalid-response"},
		{errors.New("semantic provider returned an error"), "provider-error"},
	} {
		t.Run(fixture.err.Error(), func(t *testing.T) {
			if got := geminiCompactFailureReason04384(fixture.err); got != fixture.want {
				t.Fatalf("Compact failure reason = %q, want %q", got, fixture.want)
			}
		})
	}
}
