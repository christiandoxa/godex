package proxy

import (
	"context"
	"testing"
	"time"
)

func TestProdex04384DetachedClosingCaptureRetainsEarlierParentDeadline(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	parent, cancelParent := context.WithDeadline(context.Background(), deadline)
	defer cancelParent()
	original, cancelOriginal := withRequestDeadline(parent, 2*time.Hour)
	defer cancelOriginal()
	detached, cancelDetached := detachRequestDeadline(original)
	defer cancelDetached()
	cancelParent()
	scope, ok := requestDeadlineScopeFor(detached)
	if !ok || !scope.deadline.Equal(deadline) || detached.Err() != nil {
		t.Fatalf("detached capture lost its deadline or retained disconnect cancellation: scope=%v error=%v", ok, detached.Err())
	}
}
