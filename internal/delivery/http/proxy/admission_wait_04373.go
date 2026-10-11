package proxy

import (
	"context"
	"time"
)

const (
	admissionWaitInterval         = 750 * time.Millisecond
	pressureAdmissionWaitInterval = 200 * time.Millisecond
)

func waitAdmissionSignal(ctx context.Context, changed <-chan struct{}) bool {
	select {
	case <-changed:
		return true
	case <-ctx.Done():
		return false
	}
}
