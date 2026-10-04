package proxy

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"sync"
)

type admissionLane uint8

const (
	admissionLaneResponses admissionLane = iota
	admissionLaneCompact
	admissionLaneWebSocket
	admissionLaneStandard
	admissionLaneCount
)

type admissionLimits struct {
	global int
	lane   [admissionLaneCount]int
}

type admissionWaitFunc func(context.Context, <-chan struct{}) bool

type activeRequestHandler struct {
	next http.Handler
	wait admissionWaitFunc

	mu         sync.Mutex
	active     int
	laneActive [admissionLaneCount]int
	changed    chan struct{}
	limits     admissionLimits
}

func newActiveRequestHandler(next http.Handler, globalOverride int) http.Handler {
	return newActiveRequestHandlerWithLimits(
		next,
		admissionLimitsForParallelism(runtime.NumCPU(), globalOverride),
	)
}

func newActiveRequestHandlerWithLimits(next http.Handler, limits admissionLimits) http.Handler {
	if limits.global <= 0 {
		limits = admissionLimitsForParallelism(runtime.NumCPU(), 0)
	}
	for lane := admissionLane(0); lane < admissionLaneCount; lane++ {
		if limits.lane[lane] <= 0 || limits.lane[lane] > limits.global {
			limits.lane[lane] = limits.global
		}
	}
	return &activeRequestHandler{
		next: next, wait: waitAdmissionSignal,
		changed: make(chan struct{}), limits: limits,
	}
}

func waitAdmissionSignal(ctx context.Context, changed <-chan struct{}) bool {
	select {
	case <-changed:
		return true
	case <-ctx.Done():
		return false
	}
}

func defaultActiveRequestLimit() int {
	return activeRequestLimitForParallelism(runtime.NumCPU())
}

func activeRequestLimitForParallelism(parallelism int) int {
	return admissionLimitsForParallelism(parallelism, 0).global
}

func admissionLimitsForParallelism(parallelism, globalOverride int) admissionLimits {
	workerCount, longLivedWorkerCount := admissionWorkerCounts(parallelism)
	globalLimit := globalOverride
	if globalLimit <= 0 {
		globalLimit = clampAdmissionLimit(workerCount+longLivedWorkerCount*3, 64, 512)
	}
	responses := min(globalLimit, clampAdmissionLimit(globalLimit*3/4, 4, globalLimit))
	compact := min(globalLimit, clampAdmissionLimit(globalLimit/4, 2, 6))
	websocket := min(globalLimit, max(longLivedWorkerCount, 2))
	standard := min(globalLimit, clampAdmissionLimit(workerCount*2, 8, 24))
	return admissionLimits{
		global: globalLimit,
		lane: [admissionLaneCount]int{
			admissionLaneResponses: responses,
			admissionLaneCompact:   compact,
			admissionLaneWebSocket: websocket,
			admissionLaneStandard:  standard,
		},
	}
}

func admissionWorkerCounts(parallelism int) (int, int) {
	if parallelism <= 0 {
		parallelism = 4
	}
	workerCount := clampAdmissionLimit(parallelism, 4, 12)
	longLivedWorkerCount := 24
	if parallelism < 12 {
		longLivedWorkerCount = clampAdmissionLimit(parallelism*2, 8, 24)
	}
	return workerCount, longLivedWorkerCount
}

func clampAdmissionLimit(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func admissionLaneForRequest(request *http.Request) admissionLane {
	if isWebSocketUpgradeRequest(request) {
		return admissionLaneWebSocket
	}
	path := strings.TrimRight(request.URL.Path, "/")
	if strings.HasSuffix(path, "/responses/compact") {
		return admissionLaneCompact
	}
	if strings.HasSuffix(path, "/responses") {
		return admissionLaneResponses
	}
	return admissionLaneStandard
}

func (handler *activeRequestHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	lane := admissionLaneForRequest(request)
	if !handler.acquire(request.Context(), lane) {
		return
	}
	defer handler.release(lane)
	handler.next.ServeHTTP(writer, request)
}

func (handler *activeRequestHandler) acquire(ctx context.Context, lane admissionLane) bool {
	for {
		handler.mu.Lock()
		if handler.active < handler.limits.global &&
			handler.laneActive[lane] < handler.limits.lane[lane] {
			handler.active++
			handler.laneActive[lane]++
			handler.mu.Unlock()
			return true
		}
		changed := handler.changed
		handler.mu.Unlock()

		if !handler.wait(ctx, changed) {
			return false
		}
	}
}

func (handler *activeRequestHandler) release(lane admissionLane) {
	handler.mu.Lock()
	if handler.active > 0 {
		handler.active--
	}
	if handler.laneActive[lane] > 0 {
		handler.laneActive[lane]--
	}
	close(handler.changed)
	handler.changed = make(chan struct{})
	handler.mu.Unlock()
}
