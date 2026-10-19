package proxy

import (
	"net/http"
	"strings"
)

func (handler *activeRequestHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/__godex/runtime/") || strings.HasPrefix(request.URL.Path, "/__prodex/runtime/") {
		handler.next.ServeHTTP(writer, request)
		return
	}
	requestContext, cancel := withRequestDeadline(request.Context(), handler.requestBudget)
	defer cancel()
	request = request.WithContext(requestContext)
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
		contextErr := request.Context().Err()
		if requestDeadlineTimedOut(request.Context(), contextErr) {
			writeProxyError(writer, request.URL.Path, http.StatusGatewayTimeout, requestRecoveryTimeoutMessage)
		} else if contextErr == nil {
			handler.markLocalOverload()
			writeLocalAdmissionOverloadResponse(writer, lane, isWebSocketUpgradeRequest(request))
		}
		return
	}
	defer handler.release(lane)
	handler.next.ServeHTTP(writer, request)
}
