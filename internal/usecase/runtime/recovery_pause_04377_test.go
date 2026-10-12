package runtime

import (
	"context"
	"errors"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	"testing"
	"time"
)

func TestProdex04377PauseSuppressesFreshLimit(t *testing.T) {
	process := &cancellationMonitorProcess{started: make(chan struct{})}
	runner := cancellationMonitorRunner(process)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reads := 0
	err, triggered := runner.RunWithGoalRecoveryMonitor(ctx, "", nil, RuntimeLaunchOptions{}, func(context.Context) (runtimemodel.GoalRecoveryState, bool) {
		reads++
		switch reads {
		case 1:
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "active"}, true
		case 2:
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "paused"}, true
		case 3:
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "usage_limited", UpdatedAt: time.Now().UnixMilli() + 1}, true
		default:
			cancel()
			return runtimemodel.GoalRecoveryState{SessionID: "session", Status: "active"}, true
		}
	})
	if triggered || !errors.Is(err, context.Canceled) {
		t.Fatalf("paused goal changed: triggered=%t err=%v reads=%d", triggered, err, reads)
	}
}
