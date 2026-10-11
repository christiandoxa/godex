package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestAdmissionWaitUsesBoundedPollIntervals(t *testing.T) {
	for _, test := range []struct {
		name     string
		pressure bool
		want     time.Duration
	}{
		{name: "normal", want: admissionWaitInterval},
		{name: "pressure", pressure: true, want: pressureAdmissionWaitInterval},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), admissionLimits{
				global: 1,
				lane:   [admissionLaneCount]int{1, 1, 1, 1},
			}).(*activeRequestHandler)
			handler.pressureSnapshot = func() AdmissionPressure {
				return AdmissionPressure{LocalOverload: test.pressure}
			}
			handler.active = 1
			handler.laneActive[admissionLaneResponses] = 1

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			waitEntered := make(chan time.Duration, 1)
			handler.wait = func(waitCtx context.Context, changed <-chan struct{}) bool {
				deadline, ok := waitCtx.Deadline()
				if !ok {
					cancel()
					return false
				}
				waitEntered <- time.Until(deadline)
				return waitAdmissionSignal(waitCtx, changed)
			}

			result := make(chan bool, 1)
			go func() { result <- handler.acquire(ctx, admissionLaneResponses) }()
			select {
			case remaining := <-waitEntered:
				if remaining < test.want-50*time.Millisecond || remaining > test.want+50*time.Millisecond {
					t.Fatalf("wait interval=%s, want about %s", remaining, test.want)
				}
			case <-time.After(time.Second):
				t.Fatal("admission waiter did not enter bounded wait")
			}
			cancel()
			select {
			case acquired := <-result:
				if acquired {
					t.Fatal("canceled admission waiter acquired capacity")
				}
			case <-time.After(time.Second):
				t.Fatal("canceled admission waiter did not finish")
			}
		})
	}
}
