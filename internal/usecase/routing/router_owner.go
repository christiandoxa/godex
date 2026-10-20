package routing

import (
	"context"
	"errors"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) resolveOwner(
	ctx context.Context,
	keys affinityKeys,
	selection quotamodel.Selection,
) (string, func() error, error) {
	ownerKeys := keys
	if selection.RouteKind == quotamodel.RouteKindWebSocket &&
		ownerKeys.previous != "" && ownerKeys.turn != "" {
		// Tagged 0.438.3 combines previous-response and Compact-origin turn
		// lineage evidence. A direct previous owner must not authorize an
		// inference when another profile owns the verified Compact turn.
		previousOwner, err := router.affinity.owner(ctx, affinityKeys{previous: ownerKeys.previous}, router.now())
		if err != nil {
			return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
		}
		aliasOwner, err := router.affinity.owner(ctx,
			affinityKeys{turn: "__compact_turn_state__:" + ownerKeys.turn}, router.now())
		if err != nil {
			return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
		}
		if previousOwner != "" && aliasOwner != "" && previousOwner != aliasOwner {
			return "", nil, &proxymodel.Error{
				StatusCode: http.StatusServiceUnavailable, Message: upstreamTransportFailureMessage,
			}
		}
	}
	if selection.RouteKind == quotamodel.RouteKindWebSocket && ownerKeys.previous == "" && ownerKeys.turn != "" {
		// Tagged Prodex 0.438.3 resolves Compact's separate turn lineage
		// even while the receiving socket retains the WebSocket quota route.
		// A known direct owner and a different verified lineage owner
		// conflict; only an unbound turn may adopt the Compact namespace.
		directOwner, err := router.affinity.owner(ctx, affinityKeys{turn: ownerKeys.turn}, router.now())
		if err != nil {
			return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
		}
		alias := "__compact_turn_state__:" + ownerKeys.turn
		aliasOwner, err := router.affinity.owner(ctx, affinityKeys{turn: alias}, router.now())
		if err != nil {
			return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
		}
		if directOwner != "" && aliasOwner != "" && directOwner != aliasOwner {
			// Tagged 0.438.3 resolves this as a hard binding conflict.
			// Fail before dispatch; neither owner may infer on the other's
			// durable continuation, even with reconstructable full history.
			return "", nil, &proxymodel.Error{
				StatusCode: http.StatusServiceUnavailable, Message: upstreamTransportFailureMessage,
			}
		}
		if directOwner == "" && aliasOwner != "" {
			ownerKeys.turn = alias
		}
	}
	if selection.RouteKind == quotamodel.RouteKindWebSocket && ownerKeys.previous == "" &&
		ownerKeys.turn == "" && ownerKeys.thread == "" && ownerKeys.session != "" {
		// Prodex 0.438.3 also consults verified Compact *session*
		// lineage on a WebSocket request. Existing direct session owners
		// outrank this alias; the WebSocket route remains quota-soft.
		sessionID := ownerKeys.session
		directOwner, err := router.affinity.owner(ctx, affinityKeys{session: sessionID}, router.now())
		if errors.Is(err, errConflictingAccountAffinity) {
			// A verified conflict is not a usable *soft* session preference.
			// Keep the original key for post-commit verified observations, but
			// select from Ready profiles rather than returning hard-affinity 409.
			directOwner = ""
			ownerKeys.session = ""
		} else if err != nil {
			return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
		}
		if directOwner == "" {
			alias := "__compact_session__:" + sessionID
			aliasOwner, err := router.affinity.owner(ctx, affinityKeys{session: alias}, router.now())
			if errors.Is(err, errConflictingAccountAffinity) {
				aliasOwner = ""
			} else if err != nil {
				return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
			}
			if aliasOwner != "" {
				ownerKeys.session = alias
			}
		}
	}
	if selection.RouteKind == quotamodel.RouteKindResponses &&
		ownerKeys.previous == "" && ownerKeys.turn == "" &&
		ownerKeys.thread == "" && ownerKeys.session != "" {
		// Tagged Prodex 0.438.3 filters the conflict sentinel from the
		// *soft* Responses session preference. A fresh eligible account may
		// handle a self-contained request; preserve the original session key
		// for verified post-commit observations and durable conflict state.
		_, err := router.affinity.owner(ctx, affinityKeys{session: ownerKeys.session}, router.now())
		if errors.Is(err, errConflictingAccountAffinity) {
			ownerKeys.session = ""
		} else if err != nil {
			return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
		}
	}
	if selection.RouteKind == quotamodel.RouteKindCompact {
		if ownerKeys.previous != "" {
			ownerKeys.session = ""
		} else if ownerKeys.turn != "" {
			turnOwner, err := router.affinity.owner(ctx, affinityKeys{turn: ownerKeys.turn}, router.now())
			if err != nil {
				return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
			}
			if turnOwner != "" {
				ownerKeys.session = ""
			}
		}
	} else if selection.RouteKind == quotamodel.RouteKindResponses || selection.RouteKind == quotamodel.RouteKindWebSocket {
		if ownerKeys.previous != "" || ownerKeys.turn != "" || ownerKeys.thread != "" {
			ownerKeys.session = ""
		}
	}
	owner, err := router.affinity.owner(ctx, ownerKeys, router.now())
	if err != nil {
		return "", nil, &proxymodel.Error{StatusCode: 409, Message: "request contains conflicting conversation affinity"}
	}
	var durableRelease func() error
	if owner == "" && router.affinity.repository != nil && stableConversation(keys) {
		durableRelease, err = router.affinity.repository.AcquireConversation(ctx)
		if err != nil {
			return "", nil, err
		}
		owner, err = router.affinity.owner(ctx, ownerKeys, router.now())
		if err != nil {
			_ = durableRelease()
			return "", nil, err
		}
	}
	if owner == "" && opaqueContinuation(keys, selection) &&
		!allowsUnboundResponsesContinuation(keys, selection) &&
		!allowsUnboundWebSocketDiscovery(keys, selection) &&
		(keys.previous == "" || !router.hasPreviousResponseFailure(keys.previous, selection)) {
		if durableRelease != nil {
			_ = durableRelease()
		}
		return "", nil, &proxymodel.Error{StatusCode: 409, Message: "continuation owner is unknown; continuity was preserved"}
	}
	return owner, durableRelease, nil
}

// Prodex 0.437.9 permits one selected owner for unbound
// WebSocket previous-response/turn-state context. Bounded retries may target
// that same owner, but must never cross profiles without the chain.
func allowsUnboundWebSocketDiscovery(keys affinityKeys, selection quotamodel.Selection) bool {
	return selection.RouteKind == quotamodel.RouteKindWebSocket &&
		(keys.previous != "" || keys.turn != "")
}

func stableConversation(keys affinityKeys) bool {
	return keys.thread != "" || keys.session != ""
}

func opaqueContinuation(keys affinityKeys, selection quotamodel.Selection) bool {
	if keys.previous != "" {
		return true
	}
	return keys.turn != "" && selection.RouteKind != quotamodel.RouteKindCompact
}

// Responses can discover an unbound previous-response owner by making the
// bounded upstream attempt. Other transports keep unknown opaque affinity
// fail-closed because they cannot safely replay it on a fresh profile.
func allowsUnboundResponsesContinuation(keys affinityKeys, selection quotamodel.Selection) bool {
	return selection.RouteKind == quotamodel.RouteKindResponses && keys.previous != ""
}

func (router *Router) routeRequest(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	owner string,
	keys *affinityKeys,
) (proxymodel.Forwarded, error) {
	if owner != "" {
		if keys != nil && keys.hasSoftSessionAffinity(request.QuotaSelection) &&
			router.softSessionOwnerBlocked(accounts, owner, request.QuotaSelection) {
			return router.forwardFresh(ctx, request, accounts)
		}
		return router.forwardBound(ctx, request, accounts, owner, keys)
	}
	if request.WebSocketMessage && keys != nil &&
		allowsUnboundWebSocketDiscovery(*keys, request.QuotaSelection) {
		return router.forwardUnboundWebSocketDiscovery(ctx, request, accounts, keys)
	}
	return router.forwardFresh(ctx, request, accounts)
}
