package proxy

import (
	"net/http"
	"runtime"
)

type activeRequestHandler struct {
	next  http.Handler
	slots chan struct{}
}

func newActiveRequestHandler(next http.Handler, limit int) http.Handler {
	if limit <= 0 {
		limit = defaultActiveRequestLimit()
	}
	return &activeRequestHandler{next: next, slots: make(chan struct{}, limit)}
}

func defaultActiveRequestLimit() int {
	return activeRequestLimitForParallelism(runtime.NumCPU())
}

func activeRequestLimitForParallelism(parallelism int) int {
	if parallelism <= 0 {
		parallelism = 4
	}
	workerCount := clampAdmissionLimit(parallelism, 4, 12)
	longLivedWorkerCount := 24
	if parallelism < 12 {
		longLivedWorkerCount = clampAdmissionLimit(parallelism*2, 8, 24)
	}
	return clampAdmissionLimit(workerCount+longLivedWorkerCount*3, 64, 512)
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

func (handler *activeRequestHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	select {
	case handler.slots <- struct{}{}:
		defer func() { <-handler.slots }()
	case <-request.Context().Done():
		return
	}
	handler.next.ServeHTTP(writer, request)
}
