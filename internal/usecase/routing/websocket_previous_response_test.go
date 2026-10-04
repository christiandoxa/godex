package routing

import (
	"testing"
	"time"
)

func TestWebSocketPreviousResponsePlannerMatchesProdex04354(t *testing.T) {
	for _, test := range []struct {
		name             string
		input            websocketPreviousResponsePlanInput
		wantDelay        time.Duration
		wantRetryOwner   bool
		wantStale        bool
		wantLocked       bool
		wantFreshBlocked bool
	}{
		{
			name: "turn state retry 75ms",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnStateRetry: true, retryIndex: 0,
			},
			wantDelay: 75 * time.Millisecond, wantRetryOwner: true,
		},
		{
			name: "turn state retry 200ms",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnStateRetry: true, retryIndex: 1,
			},
			wantDelay: 200 * time.Millisecond, wantRetryOwner: true,
		},
		{
			name: "turn state retry 500ms",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnStateRetry: true, retryIndex: 2,
			},
			wantDelay: 500 * time.Millisecond, wantRetryOwner: true,
		},
		{
			name: "turn state exhausted fails closed",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnStateRetry: true, retryIndex: 3,
			},
			wantStale: true,
		},
		{
			name: "semantic locked affinity retries without turn state",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, requestRequiresPreviousAffinity: true, retryIndex: 0,
			},
			wantDelay: 75 * time.Millisecond, wantRetryOwner: true, wantLocked: true,
		},
		{
			name: "semantic locked affinity exhausted fails closed",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, requestRequiresPreviousAffinity: true, retryIndex: 3,
			},
			wantStale: true, wantLocked: true,
		},
		{
			name: "trusted previous owner alone does not create retry reason",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, trustedPreviousAffinity: true, retryIndex: 0,
			},
			wantStale: true, wantLocked: true,
		},
		{
			name: "trusted previous owner with explicit request turn state is not locked by trust",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, trustedPreviousAffinity: true,
				requestTurnStatePresent: true, retryIndex: 0,
			},
			wantStale: true, wantFreshBlocked: true,
		},
		{
			name: "unbound continuation without turn state fails closed and blocks fresh fallback",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, retryIndex: 0,
			},
			wantStale: true, wantFreshBlocked: true,
		},
		{
			name:  "no previous response",
			input: websocketPreviousResponsePlanInput{retryIndex: 0},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := planWebSocketPreviousResponse(test.input)
			if got.retryDelay != test.wantDelay ||
				got.retryOwner != test.wantRetryOwner ||
				got.staleContinuation != test.wantStale ||
				got.requestRequiresLockedAffinity != test.wantLocked ||
				got.freshBlockedWithoutAffinity != test.wantFreshBlocked {
				t.Fatalf("plan = %#v", got)
			}
		})
	}
}

func TestWebSocketPreviousResponseRetryScheduleIsIndependentFromImplementationConstants(t *testing.T) {
	got := []time.Duration{
		planWebSocketPreviousResponse(websocketPreviousResponsePlanInput{
			previousPresent: true, hasTurnStateRetry: true, retryIndex: 0,
		}).retryDelay,
		planWebSocketPreviousResponse(websocketPreviousResponsePlanInput{
			previousPresent: true, hasTurnStateRetry: true, retryIndex: 1,
		}).retryDelay,
		planWebSocketPreviousResponse(websocketPreviousResponsePlanInput{
			previousPresent: true, hasTurnStateRetry: true, retryIndex: 2,
		}).retryDelay,
	}
	want := []time.Duration{
		75 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("retry %d delay = %s, want %s", index, got[index], want[index])
		}
	}
}
