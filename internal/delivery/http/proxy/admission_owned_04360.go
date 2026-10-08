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

// requestHasOwnedAdmissionAffinity only inspects bounded HTTP payloads when
// one non-global lane is saturated. Reading the body at admission time is
// safe only when every inspected byte is replayed unchanged to the ordinary
// handler. An overlarge/invalid body never grants an admission bypass.
func (handler *activeRequestHandler) requestHasOwnedAdmissionAffinity(request *http.Request, lane admissionLane) bool {
	return handler.inspectAdmissionOwner(request, lane, false, handler.ownedAdmission)
}

func (handler *activeRequestHandler) requestHasCompactPressureOwner(request *http.Request) bool {
	return handler.inspectAdmissionOwner(request, admissionLaneCompact, true, handler.compactOwner)
}

func (handler *activeRequestHandler) inspectAdmissionOwner(
	request *http.Request, lane admissionLane, force bool,
	owns func(context.Context, admissionLane, http.Header, []byte) bool,
) bool {
	if owns == nil || request.Context().Err() != nil || lane == admissionLaneStandard {
		return false
	}
	if !force {
		handler.mu.Lock()
		saturated := handler.active < handler.limits.global &&
			handler.laneActive[lane] >= handler.limits.lane[lane]
		handler.mu.Unlock()
		if !saturated {
			return false
		}
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
			return false
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
			return false
		}
		payload = prefix
	}
	return owns(request.Context(), lane, request.Header, payload)
}

func (proxy *Proxy) hasVerifiedCompactPressureOwner(ctx context.Context, _ admissionLane, headers http.Header, body []byte) bool {
	return proxy != nil && proxy.router != nil && proxy.router.HasVerifiedCompactPressureOwner(ctx, headers, body)
}
