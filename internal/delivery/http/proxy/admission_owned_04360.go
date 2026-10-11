package proxy

import (
	"bytes"
	"context"
	"io"
	"net/http"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type admissionReplayedBody struct {
	io.Reader
	io.Closer
}

func (proxy *Proxy) hasVerifiedAdmissionOwner(ctx context.Context, lane admissionLane, headers http.Header, body []byte) bool {
	if proxy == nil || proxy.router == nil {
		return false
	}
	var kind quotamodel.RouteKind
	switch lane {
	case admissionLaneResponses:
		kind = quotamodel.RouteKindResponses
	case admissionLaneCompact:
		kind = quotamodel.RouteKindCompact
	case admissionLaneWebSocket:
		kind = quotamodel.RouteKindWebSocket
	default:
		return false
	}
	return proxy.router.HasVerifiedAdmissionOwner(ctx, kind, headers, body)
}

func (handler *activeRequestHandler) requestHasCompactPressureOwner(request *http.Request) bool {
	return handler.inspectAdmissionOwner(request, admissionLaneCompact, handler.compactOwner)
}

func (handler *activeRequestHandler) admissionOwnerProjection(
	request *http.Request, lane admissionLane,
) admissionOwnerProjection {
	owns := handler.ownedAdmission
	if owns == nil {
		return nil
	}
	var payload []byte
	inspected := false
	valid := false
	return func() bool {
		if !handler.admissionLaneSaturated(lane) {
			return false
		}
		if !inspected {
			payload, valid = handler.inspectAdmissionPayload(request, lane)
			inspected = true
		}
		return valid && owns(request.Context(), lane, request.Header, payload)
	}
}

func (handler *activeRequestHandler) admissionLaneSaturated(lane admissionLane) bool {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.active < handler.limits.global && handler.laneActive[lane] >= handler.limits.lane[lane]
}

func (handler *activeRequestHandler) inspectAdmissionOwner(
	request *http.Request, lane admissionLane,
	owns func(context.Context, admissionLane, http.Header, []byte) bool,
) bool {
	if owns == nil {
		return false
	}
	payload, valid := handler.inspectAdmissionPayload(request, lane)
	if !valid {
		return false
	}
	return owns(request.Context(), lane, request.Header, payload)
}

func (handler *activeRequestHandler) inspectAdmissionPayload(
	request *http.Request, lane admissionLane,
) ([]byte, bool) {
	if request.Context().Err() != nil || lane == admissionLaneStandard {
		return nil, false
	}
	var payload []byte
	// WebSocket upgrades are validated without consuming handshake bytes.
	if lane != admissionLaneWebSocket && request.Body != nil && request.Body != http.NoBody {
		limit := handler.inspectLimit
		if limit <= 0 {
			limit = 64 << 10
		}
		// Do not load oversized or unbounded requests into memory.
		if request.ContentLength > limit {
			return nil, false
		}
		original := request.Body
		prefix, err := io.ReadAll(io.LimitReader(original, limit+1))
		if len(prefix) > 0 {
			request.Body = &admissionReplayedBody{
				Reader: io.MultiReader(bytes.NewReader(prefix), original),
				Closer: original,
			}
		}
		if err != nil || int64(len(prefix)) > limit {
			return nil, false
		}
		payload = prefix
	}
	return payload, true
}

func (proxy *Proxy) hasVerifiedCompactPressureOwner(ctx context.Context, _ admissionLane, headers http.Header, body []byte) bool {
	return proxy != nil && proxy.router != nil && proxy.router.HasVerifiedCompactPressureOwner(ctx, headers, body)
}
