package routing

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

const routeDecisionTraceSchemaVersion = 1
const routeDecisionMaxIdentifierBytes = 96

type routeDecisionTrace struct {
	schemaVersion     int
	route             string
	requestedModel    string
	resolvedModel     string
	selectedCandidate string
	terminalOutcome   string
	terminalReason    string
}

func (router *Router) recordRouteDecisionSelected(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) {
	router.recordRouteDecision(ctx, request, routeDecisionTrace{
		schemaVersion:     routeDecisionTraceSchemaVersion,
		route:             routeDecisionRouteLabel(request.QuotaSelection.RouteKind),
		requestedModel:    routeDecisionIdentifier(request.QuotaSelection.RequestedModel),
		resolvedModel:     routeDecisionIdentifier(request.QuotaSelection.RequestedModel),
		selectedCandidate: routeDecisionIdentifier(account.ID),
		terminalOutcome:   "selected",
	})
}

func (router *Router) recordRouteDecisionNoCandidate(
	ctx context.Context,
	request proxymodel.Request,
) {
	router.recordRouteDecision(ctx, request, routeDecisionTrace{
		schemaVersion:   routeDecisionTraceSchemaVersion,
		route:           routeDecisionRouteLabel(request.QuotaSelection.RouteKind),
		requestedModel:  routeDecisionIdentifier(request.QuotaSelection.RequestedModel),
		resolvedModel:   routeDecisionIdentifier(request.QuotaSelection.RequestedModel),
		terminalOutcome: "no_candidate",
	})
}

func (router *Router) recordRouteDecision(
	ctx context.Context,
	request proxymodel.Request,
	trace routeDecisionTrace,
) {
	if router == nil || router.activity == nil {
		return
	}
	payload := map[string]any{
		"schema_version":   trace.schemaVersion,
		"route":            trace.route,
		"terminal_outcome": trace.terminalOutcome,
	}
	if trace.requestedModel != "" {
		payload["requested_model"] = trace.requestedModel
	}
	if trace.resolvedModel != "" {
		payload["resolved_model"] = trace.resolvedModel
	}
	if trace.selectedCandidate != "" {
		payload["selected_candidate"] = trace.selectedCandidate
	}
	if trace.terminalReason != "" {
		payload["terminal_reason"] = trace.terminalReason
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	requestID := ""
	fields := map[string]string{
		"schema_version": strconv.Itoa(trace.schemaVersion),
		"route":          trace.route,
		"outcome":        trace.terminalOutcome,
		"trace":          string(encoded),
	}
	if request.RequestID != 0 {
		requestID = strconv.FormatUint(request.RequestID, 10)
		fields["request"] = requestID
	}
	router.recordRuntimeMarker(ctx, runtimemodel.Event{
		Kind:      "route_decision",
		RequestID: requestID,
		Path:      request.Path,
		Fields:    fields,
	})
}

func routeDecisionRouteLabel(kind quotamodel.RouteKind) string {
	switch kind {
	case quotamodel.RouteKindResponses:
		return "responses"
	case quotamodel.RouteKindCompact:
		return "responses_compact"
	case quotamodel.RouteKindWebSocket:
		return "websocket"
	default:
		return "standard"
	}
}

func routeDecisionIdentifier(value string) string {
	safe, _ := routeDecisionSafeIdentifier(value)
	return safe
}

func routeDecisionSafeIdentifier(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) <= routeDecisionMaxIdentifierBytes {
		return value, false
	}
	end := routeDecisionMaxIdentifierBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end], end < len(value)
}
