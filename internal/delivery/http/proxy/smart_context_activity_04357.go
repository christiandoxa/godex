package proxy

import (
	"context"
	"strconv"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func (proxy *Proxy) recordSmartContextResult(
	ctx context.Context,
	requestID uint64,
	path string,
	websocket bool,
	bodyBytesBefore int,
	result smartContextRewrite,
) {
	if proxy == nil || proxy.activity == nil {
		return
	}
	transport := "http"
	if websocket {
		transport = "websocket"
	}
	route := smartContextRouteLabel(quotaSelection(path, websocket, result.Body).RouteKind)
	request := strconv.FormatUint(requestID, 10)

	if result.FallbackReason != "" {
		proxy.recordActivity(ctx, runtimemodel.Event{
			Kind:      "smart_context_prepare_fallback",
			RequestID: request,
			Path:      path,
			Fields: map[string]string{
				"transport":  transport,
				"route":      route,
				"profile":    "-",
				"reason":     result.FallbackReason,
				"decision":   "pass_through",
				"body_bytes": strconv.Itoa(bodyBytesBefore),
			},
		})
		return
	}
	if result.Decision == "" {
		return
	}

	fallbackReason := "-"
	if result.Decision == "self_check_passthrough" && result.SelfCheck != "" {
		fallbackReason = result.SelfCheck
	}
	bodyBytesAfter := len(result.Body)
	if result.Decision == "self_check_passthrough" {
		bodyBytesAfter = bodyBytesBefore
	}
	proxy.recordActivity(ctx, runtimemodel.Event{
		Kind:      "smart_context_autopilot",
		RequestID: request,
		Path:      path,
		Fields: map[string]string{
			"transport":         transport,
			"route":             route,
			"tier":              "exact",
			"decision":          result.Decision,
			"reasons":           "-",
			"body_bytes_before": strconv.Itoa(bodyBytesBefore),
			"body_bytes_after":  strconv.Itoa(bodyBytesAfter),
			"self_check":        result.SelfCheck,
			"rewrite_kind":      result.Decision,
			"rewrite_status":    result.SelfCheck,
			"fallback_reason":   fallbackReason,
		},
	})
}

func smartContextRouteLabel(kind quotamodel.RouteKind) string {
	switch kind {
	case quotamodel.RouteKindResponses:
		return "responses"
	case quotamodel.RouteKindCompact:
		return "compact"
	case quotamodel.RouteKindWebSocket:
		return "websocket"
	default:
		return "standard"
	}
}
