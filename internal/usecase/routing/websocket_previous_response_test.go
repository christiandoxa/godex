package routing

import (
	"testing"
	"time"
)

func TestWebSocketPreviousResponsePlannerMatchesProdex04353(t *testing.T) {
	for _, test := range []struct {
		name           string
		input          websocketPreviousResponsePlanInput
		wantDelay      time.Duration
		wantRetryOwner bool
		wantStale      bool
	}{
		{
			name: "turn state retry 75ms",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnState: true, retryIndex: 0,
			},
			wantDelay: 75 * time.Millisecond, wantRetryOwner: true,
		},
		{
			name: "turn state retry 200ms",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnState: true, retryIndex: 1,
			},
			wantDelay: 200 * time.Millisecond, wantRetryOwner: true,
		},
		{
			name: "turn state retry 500ms",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnState: true, retryIndex: 2,
			},
			wantDelay: 500 * time.Millisecond, wantRetryOwner: true,
		},
		{
			name: "turn state exhausted fails closed",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, hasTurnState: true, retryIndex: 3,
			},
			wantStale: true,
		},
		{
			name: "locked affinity retries without turn state",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, lockedAffinity: true, retryIndex: 0,
			},
			wantDelay: 75 * time.Millisecond, wantRetryOwner: true,
		},
		{
			name: "locked affinity exhausted fails closed",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, lockedAffinity: true, retryIndex: 3,
			},
			wantStale: true,
		},
		{
			name: "unlocked continuation without turn state fails closed",
			input: websocketPreviousResponsePlanInput{
				previousPresent: true, retryIndex: 0,
			},
			wantStale: true,
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
				got.staleContinuation != test.wantStale {
				t.Fatalf("plan = %#v", got)
			}
		})
	}
}

func TestWebSocketPreviousResponseRetryScheduleIsIndependentFromImplementationConstants(t *testing.T) {
	got := []time.Duration{
		planWebSocketPreviousResponse(websocketPreviousResponsePlanInput{
			previousPresent: true, hasTurnState: true, retryIndex: 0,
		}).retryDelay,
		planWebSocketPreviousResponse(websocketPreviousResponsePlanInput{
			previousPresent: true, hasTurnState: true, retryIndex: 1,
		}).retryDelay,
		planWebSocketPreviousResponse(websocketPreviousResponsePlanInput{
			previousPresent: true, hasTurnState: true, retryIndex: 2,
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
