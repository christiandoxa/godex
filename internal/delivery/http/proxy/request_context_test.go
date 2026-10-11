package proxy

import (
	"context"
	"testing"
	"time"
)

func TestRequestContextAfterBodyPropagatesCancellationDuringGrace(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	forward, stop := requestContextAfterBody(parent)
	defer stop()
	cancelParent()

	select {
	case <-forward.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("forwarding context did not observe cancellation")
	}
}
