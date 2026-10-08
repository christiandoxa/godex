package proxy

import (
	"context"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
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

	mu                 sync.Mutex
	active             int
	laneActive         [admissionLaneCount]int
	changed            chan struct{}
	limits             admissionLimits
	activity           activityRecorder
	ownedAdmission     func(context.Context, admissionLane, http.Header, []byte) bool
	compactOwner       func(context.Context, admissionLane, http.Header, []byte) bool
	pressureSnapshot   func() AdmissionPressure
	localOverloadUntil time.Time
	now                func() time.Time
	inspectLimit       int64

	admissionsTotal            [admissionLaneCount]uint64
	releasesTotal              [admissionLaneCount]uint64
	globalLimitRejectionsTotal [admissionLaneCount]uint64
	laneLimitRejectionsTotal   [admissionLaneCount]uint64
	releaseUnderflowsTotal     [admissionLaneCount]uint64
	activeReleaseUnderflows    uint64
	waitTotalNS                uint64
	waitCount                  uint64
	waitMaxNS                  uint64
}

func newActiveRequestHandler(next http.Handler, globalOverride int) http.Handler {
	return newActiveRequestHandlerWithLimits(
		next, admissionLimitsForParallelism(runtime.NumCPU(), globalOverride),
	)
}

func newActiveRequestHandlerWithRecorder(next http.Handler, globalOverride int, activity activityRecorder) http.Handler {
	return newActiveRequestHandlerWithLimitsAndRecorder(
		next, admissionLimitsForParallelism(runtime.NumCPU(), globalOverride), activity,
	)
}

func newActiveRequestHandlerWithLimits(next http.Handler, limits admissionLimits) http.Handler {
	return newActiveRequestHandlerWithLimitsAndRecorder(next, limits, nil)
}

func newActiveRequestHandlerWithLimitsAndRecorder(next http.Handler, limits admissionLimits, activity activityRecorder) http.Handler {
	if limits.global <= 0 {
		limits = admissionLimitsForParallelism(runtime.NumCPU(), 0)
	}
	for lane := admissionLane(0); lane < admissionLaneCount; lane++ {
		if limits.lane[lane] <= 0 || limits.lane[lane] > limits.global {
			limits.lane[lane] = limits.global
		}
	}
	handler := &activeRequestHandler{
		next: next, wait: waitAdmissionSignal,
		changed: make(chan struct{}), limits: limits, activity: activity,
	}
	if proxy, ok := next.(*Proxy); ok && proxy.router != nil {
		handler.ownedAdmission = proxy.hasVerifiedAdmissionOwner
		handler.compactOwner = proxy.hasVerifiedCompactPressureOwner
		handler.pressureSnapshot = proxy.pressureSnapshot
		handler.inspectLimit = proxy.maxRequest
	}
	return handler
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
	if strings.HasPrefix(request.URL.Path, "/__godex/runtime/") || strings.HasPrefix(request.URL.Path, "/__prodex/runtime/") {
		handler.next.ServeHTTP(writer, request)
		return
	}
	lane := admissionLaneForRequest(request)
	if lane == admissionLaneStandard && handler.shedOptionalStartupMetadata(writer, request.URL.Path) {
		return
	}
	if lane == admissionLaneCompact && handler.pressureMode(lane) &&
		!handler.requestHasCompactPressureOwner(request) {
		writeFreshCompactPressureResponse(writer)
		return
	}
	if !handler.acquireRequest(request, lane) {
		if request.Context().Err() == nil {
			handler.markLocalOverload()
			writeLocalAdmissionOverloadResponse(writer, lane, isWebSocketUpgradeRequest(request))
		}
		return
	}
	defer handler.release(lane)
	handler.next.ServeHTTP(writer, request)
}

func (handler *activeRequestHandler) acquire(ctx context.Context, lane admissionLane) bool {
	return handler.acquireWithMetadata(ctx, lane, "", "")
}

func (handler *activeRequestHandler) acquireRequest(request *http.Request, lane admissionLane) bool {
	transport := "http"
	if isWebSocketUpgradeRequest(request) {
		transport = "websocket"
	}
	bypass := handler.requestHasOwnedAdmissionAffinity(request, lane)
	return handler.acquireWithPolicy(request.Context(), lane, request.URL.Path, transport, bypass)
}

func (handler *activeRequestHandler) acquireWithMetadata(ctx context.Context, lane admissionLane, path, transport string) bool {
	return handler.acquireWithPolicy(ctx, lane, path, transport, false)
}

func (handler *activeRequestHandler) acquireWithPolicy(ctx context.Context, lane admissionLane, path, transport string, bypassOwnedLane bool) bool {
	var waitStarted time.Time
	for {
		handler.mu.Lock()
		// Prodex admits important models/MCP bootstrap traffic even when
		// the standard lane is full, but never above the global request cap.
		priority := lane == admissionLaneStandard && startupStandardPriorityPath(path)
		if handler.active < handler.limits.global && (handler.laneActive[lane] < handler.limits.lane[lane] || priority || bypassOwnedLane) {
			handler.active++
			handler.laneActive[lane]++
			handler.admissionsTotal[lane]++
			if !waitStarted.IsZero() {
				handler.recordWaitLocked(time.Since(waitStarted))
			}
			handler.mu.Unlock()
			return true
		}
		active, globalLimit := handler.active, handler.limits.global
		laneActive, laneLimit := handler.laneActive[lane], handler.limits.lane[lane]
		if active >= globalLimit {
			handler.globalLimitRejectionsTotal[lane]++
		} else {
			handler.laneLimitRejectionsTotal[lane]++
		}
		if waitStarted.IsZero() {
			waitStarted = time.Now()
		}
		changed := handler.changed
		handler.mu.Unlock()
		handler.recordAdmissionPressure(ctx, lane, path, transport, active, globalLimit, laneActive, laneLimit)

		if !handler.wait(ctx, changed) {
			handler.mu.Lock()
			handler.recordWaitLocked(time.Since(waitStarted))
			handler.mu.Unlock()
			return false
		}
	}
}

func (handler *activeRequestHandler) recordWaitLocked(duration time.Duration) {
	if duration < 0 {
		return
	}
	ns := uint64(duration)
	handler.waitTotalNS += ns
	handler.waitCount++
	if ns > handler.waitMaxNS {
		handler.waitMaxNS = ns
	}
}

func (handler *activeRequestHandler) recordAdmissionPressure(
	ctx context.Context, lane admissionLane, path, transport string, active, globalLimit, laneActive, laneLimit int,
) {
	if handler.activity == nil {
		return
	}
	kind := "runtime_proxy_lane_limit_reached"
	fields := map[string]string{
		"transport": transport, "path": path, "lane": admissionLaneLabel(lane),
		"active": strconv.Itoa(laneActive), "limit": strconv.Itoa(laneLimit),
	}
	if active >= globalLimit {
		kind = "runtime_proxy_active_limit_reached"
		fields["active"], fields["limit"] = strconv.Itoa(active), strconv.Itoa(globalLimit)
	}
	_ = handler.activity.Record(context.WithoutCancel(ctx), runtimemodel.Event{Kind: kind, Path: path, Fields: fields})
}

func admissionLaneLabel(lane admissionLane) string {
	switch lane {
	case admissionLaneResponses:
		return "responses"
	case admissionLaneCompact:
		return "compact"
	case admissionLaneWebSocket:
		return "websocket"
	case admissionLaneStandard:
		return "standard"
	default:
		return "unknown"
	}
}

func (handler *activeRequestHandler) release(lane admissionLane) {
	handler.mu.Lock()
	if handler.active > 0 {
		handler.active--
	} else {
		handler.activeReleaseUnderflows++
	}
	if handler.laneActive[lane] > 0 {
		handler.laneActive[lane]--
		handler.releasesTotal[lane]++
	} else {
		handler.releaseUnderflowsTotal[lane]++
	}
	close(handler.changed)
	handler.changed = make(chan struct{})
	handler.mu.Unlock()
}

type admissionWaitSnapshot struct {
	TotalNS uint64
	Count   uint64
	MaxNS   uint64
}

type admissionLaneSnapshot struct {
	Active                     int
	Limit                      int
	AdmissionsTotal            uint64
	ReleasesTotal              uint64
	GlobalLimitRejectionsTotal uint64
	LaneLimitRejectionsTotal   uint64
	ReleaseUnderflowsTotal     uint64
}

type admissionSnapshot struct {
	GlobalLimit                    int
	Lanes                          [admissionLaneCount]admissionLaneSnapshot
	Wait                           admissionWaitSnapshot
	ActiveRequestReleaseUnderflows uint64
}

func (handler *activeRequestHandler) snapshot() admissionSnapshot {
	if handler == nil {
		return admissionSnapshot{}
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	snapshot := admissionSnapshot{
		GlobalLimit: handler.limits.global,
		Wait: admissionWaitSnapshot{
			TotalNS: handler.waitTotalNS,
			Count:   handler.waitCount,
			MaxNS:   handler.waitMaxNS,
		},
		ActiveRequestReleaseUnderflows: handler.activeReleaseUnderflows,
	}
	for lane := admissionLane(0); lane < admissionLaneCount; lane++ {
		snapshot.Lanes[lane] = admissionLaneSnapshot{
			Active:                     handler.laneActive[lane],
			Limit:                      handler.limits.lane[lane],
			AdmissionsTotal:            handler.admissionsTotal[lane],
			ReleasesTotal:              handler.releasesTotal[lane],
			GlobalLimitRejectionsTotal: handler.globalLimitRejectionsTotal[lane],
			LaneLimitRejectionsTotal:   handler.laneLimitRejectionsTotal[lane],
			ReleaseUnderflowsTotal:     handler.releaseUnderflowsTotal[lane],
		}
	}
	return snapshot
}
