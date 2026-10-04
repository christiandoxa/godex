package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdmissionLaneDefaultsMatchProdexTuningPolicy(t *testing.T) {
	for _, test := range []struct {
		name        string
		parallelism int
		override    int
		global      int
		responses   int
		compact     int
		websocket   int
		standard    int
	}{
		{name: "small host", parallelism: 4, global: 64, responses: 48, compact: 6, websocket: 8, standard: 8},
		{name: "eight cpu", parallelism: 8, global: 64, responses: 48, compact: 6, websocket: 16, standard: 16},
		{name: "ten cpu", parallelism: 10, global: 70, responses: 52, compact: 6, websocket: 20, standard: 20},
		{name: "twelve cpu", parallelism: 12, global: 84, responses: 63, compact: 6, websocket: 24, standard: 24},
		{name: "explicit global clamp", parallelism: 4, override: 8, global: 8, responses: 6, compact: 2, websocket: 8, standard: 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			limits := admissionLimitsForParallelism(test.parallelism, test.override)
			if limits.global != test.global ||
				limits.lane[admissionLaneResponses] != test.responses ||
				limits.lane[admissionLaneCompact] != test.compact ||
				limits.lane[admissionLaneWebSocket] != test.websocket ||
				limits.lane[admissionLaneStandard] != test.standard {
				t.Fatalf("limits = %#v", limits)
			}
		})
	}
}

func TestAdmissionLaneClassificationMatchesRuntimeRoutes(t *testing.T) {
	for _, test := range []struct {
		path    string
		upgrade bool
		want    admissionLane
	}{
		{path: "/backend-api/codex/responses", want: admissionLaneResponses},
		{path: "/backend-api/prodex/v0.2.99/responses", want: admissionLaneResponses},
		{path: "/backend-api/codex/responses/compact", want: admissionLaneCompact},
		{path: "/v1/chat/completions", want: admissionLaneStandard},
		{path: "/backend-api/codex/responses", upgrade: true, want: admissionLaneWebSocket},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://example.test"+test.path, nil)
		if test.upgrade {
			request.Header.Set("Upgrade", "websocket")
		}
		if got := admissionLaneForRequest(request); got != test.want {
			t.Fatalf("path %q upgrade=%t lane=%d want=%d", test.path, test.upgrade, got, test.want)
		}
	}
}

func TestLaneSaturationWaiterDoesNotReserveGlobalCapacity(t *testing.T) {
	handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), admissionLimits{
		global: 2,
		lane: [admissionLaneCount]int{
			admissionLaneResponses: 1,
			admissionLaneCompact:   1,
			admissionLaneWebSocket: 1,
			admissionLaneStandard:  1,
		},
	}).(*activeRequestHandler)

	waiting := make(chan struct{}, 1)
	handler.wait = func(ctx context.Context, changed <-chan struct{}) bool {
		select {
		case waiting <- struct{}{}:
		default:
		}
		return waitAdmissionSignal(ctx, changed)
	}
	if !handler.acquire(context.Background(), admissionLaneResponses) {
		t.Fatal("first responses admission rejected")
	}
	secondDone := make(chan bool, 1)
	go func() {
		secondDone <- handler.acquire(context.Background(), admissionLaneResponses)
	}()
	select {
	case <-waiting:
	case acquired := <-secondDone:
		handler.release(admissionLaneResponses)
		t.Fatalf("second responses admission returned before waiting: %t", acquired)
	case <-time.After(5 * time.Second):
		handler.release(admissionLaneResponses)
		t.Fatal("second responses admission never entered backpressure wait")
	}

	if !handler.acquire(context.Background(), admissionLaneStandard) {
		handler.release(admissionLaneResponses)
		t.Fatal("standard lane could not use free global capacity while responses was saturated")
	}
	handler.release(admissionLaneStandard)

	select {
	case acquired := <-secondDone:
		handler.release(admissionLaneResponses)
		if acquired {
			handler.release(admissionLaneResponses)
			t.Fatal("second responses admission bypassed saturated lane")
		}
		t.Fatal("second responses admission returned instead of waiting")
	default:
	}

	handler.release(admissionLaneResponses)
	select {
	case acquired := <-secondDone:
		if !acquired {
			t.Fatal("responses waiter did not resume after lane release")
		}
		handler.release(admissionLaneResponses)
	case <-time.After(5 * time.Second):
		t.Fatal("responses waiter did not resume after lane release")
	}
}

func TestLaneSaturationWaitHonorsCancellation(t *testing.T) {
	handler := newActiveRequestHandlerWithLimits(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), admissionLimits{
		global: 2,
		lane: [admissionLaneCount]int{
			admissionLaneResponses: 1,
			admissionLaneCompact:   1,
			admissionLaneWebSocket: 1,
			admissionLaneStandard:  1,
		},
	}).(*activeRequestHandler)
	if !handler.acquire(context.Background(), admissionLaneResponses) {
		t.Fatal("first responses admission rejected")
	}
	defer handler.release(admissionLaneResponses)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if handler.acquire(ctx, admissionLaneResponses) {
		t.Fatal("canceled lane waiter unexpectedly acquired capacity")
	}
}
