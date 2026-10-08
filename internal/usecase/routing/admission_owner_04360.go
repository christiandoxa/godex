package routing

import (
	"context"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// HasVerifiedAdmissionOwner is a fail-closed, read-only eligibility probe
// for the outer HTTP admission lane. A client-provided UUID alone never
// earns extra capacity: its response/turn/session binding must resolve to
// a currently registered managed account. This is not a quota check and
// never creates an affinity binding.
func (router *Router) HasVerifiedAdmissionOwner(ctx context.Context, lane quotamodel.RouteKind, headers http.Header, body []byte) bool {
	if router == nil || ctx.Err() != nil {
		return false
	}
	keys := requestAffinity(proxymodel.Request{Header: headers}, body)
	// Prodex bypasses a saturated responses/WebSocket lane for an
	// existing previous-response/turn owner; compact also accepts an
	// existing session owner. A free-form thread or invented session
	// is insufficient for responses.
	switch lane {
	case quotamodel.RouteKindResponses, quotamodel.RouteKindWebSocket:
		keys.session, keys.thread = "", ""
	case quotamodel.RouteKindCompact:
		keys.previous, keys.thread = "", ""
	default:
		return false
	}
	if router.hasRegisteredAdmissionOwner(ctx, keys) {
		return true
	}
	// Compact's durable lineage binding is an explicit alternate owner key.
	// Preserve the turn-state qualifier when resolving the alias.
	if lane == quotamodel.RouteKindCompact && router.compactAliasEligible(ctx, keys.session) {
		keys.session = "__compact_session__:" + keys.session
		return router.hasRegisteredAdmissionOwner(ctx, keys)
	}
	return false
}

// Prodex Compact pressure-shedding admits an existing previous-response,
// session or turn-state owner. That is wider than Compact's lane-limit
// bypass policy, which accepts only bound session/turn identity.
func (router *Router) HasVerifiedCompactPressureOwner(ctx context.Context, headers http.Header, body []byte) bool {
	if router == nil || ctx.Err() != nil {
		return false
	}
	keys := requestAffinity(proxymodel.Request{Header: headers}, body)
	keys.thread = ""
	if router.hasRegisteredAdmissionOwner(ctx, keys) {
		return true
	}
	if router.compactAliasEligible(ctx, keys.session) {
		keys.session = "__compact_session__:" + keys.session
		return router.hasRegisteredAdmissionOwner(ctx, keys)
	}
	return false
}

func (router *Router) hasRegisteredAdmissionOwner(ctx context.Context, keys affinityKeys) bool {
	if !keys.hasAffinity() {
		return false
	}
	owner, err := router.affinity.owner(ctx, keys, router.now())
	if err != nil || owner == "" {
		return false
	}
	accounts, err := router.source(ctx)
	if err != nil || ctx.Err() != nil {
		return false
	}
	for _, account := range accounts {
		if account.ID == owner {
			return true
		}
	}
	return false
}

// An alternate compact lineage must never override an already-known or
// conflicting direct session binding. Only truly missing direct sessions
// may resolve through the durable compact alias.
func (router *Router) compactAliasEligible(ctx context.Context, session string) bool {
	if session == "" {
		return false
	}
	owner, err := router.affinity.owner(ctx, affinityKeys{session: session}, router.now())
	return err == nil && owner == ""
}
