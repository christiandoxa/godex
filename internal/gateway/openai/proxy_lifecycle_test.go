package openai

import "testing"

func TestRequestLifecycleAllowsRetryOnlyBeforeCommit(t *testing.T) {
	lifecycle := &requestLifecycle{}
	if !lifecycle.canAttempt() {
		t.Fatal("new request should be uncommitted")
	}
	lifecycle.commit()
	if lifecycle.canAttempt() || lifecycle.phase != requestCommitted {
		t.Fatalf("committed lifecycle = %#v", lifecycle)
	}
	lifecycle.failAfterCommit()
	if lifecycle.phase != requestFailedAfterCommit {
		t.Fatalf("failed lifecycle = %#v", lifecycle)
	}
	lifecycle.complete()
	if lifecycle.phase != requestFailedAfterCommit {
		t.Fatalf("failed lifecycle changed to %#v", lifecycle.phase)
	}

	lifecycle = &requestLifecycle{}
	lifecycle.commit()
	lifecycle.complete()
	if lifecycle.phase != requestCompleted {
		t.Fatalf("completed lifecycle = %#v", lifecycle)
	}
}
