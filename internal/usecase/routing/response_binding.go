package routing

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) bindSuccessfulResponse(
	ctx context.Context,
	result *proxymodel.Forwarded,
	accounts []proxymodel.Account,
	keys affinityKeys,
	request proxymodel.Request,
	promptCacheKey string,
) error {
	if (rawKeyPool(accounts, "deepseek") || rawKeyPool(accounts, "gemini")) && rawKeyLocalRoute(request) {
		return nil
	}
	if result.Response.PrecommitFailure != nil && result.Response.PrecommitFailure.StaleContinuation {
		result.Failed = true
		return nil
	}
	if result.Failed || result.Response.StatusCode >= 400 {
		return nil
	}
	// The exact Mojo hard-binding component classifier applies to raw
	// request/response identifiers before any derived Compact alias is
	// constructed. Keep the original wire request unchanged.
	if !validHardBindingComponent04383(keys.previous) {
		keys.previous = ""
	}
	if !validHardBindingComponent04383(keys.turn) {
		keys.turn = ""
	}
	if !validHardBindingComponent04383(keys.session) {
		keys.session = ""
	}
	if keys.turn != "" && router.affinity.deadTurnStateContext(ctx, keys.turn, router.now()) {
		keys.turn = ""
	}
	router.clearPreviousResponseFailures(ctx, result.AccountID, keys.previous)
	router.rememberPromptCacheOwner(result.AccountID, promptCacheKey, router.now())
	selection := request.QuotaSelection
	if request.WebSocketMessage {
		previousBound := keys.previous != ""
		if result.Response.WebSocketResponseID != "" {
			keys.previous = result.Response.WebSocketResponseID
			if !validHardBindingComponent04383(keys.previous) {
				keys.previous = ""
			}
			if turnState := responseTurnStateValue(result.Response); turnState != "" {
				keys.turn = turnState
			}
		}
		bindingKeys, err := router.responseBindingKeys(ctx, keys, selection, result.AccountID)
		if err != nil {
			return err
		}
		now := router.now()
		if err := router.rememberVerifiedAccountBinding(ctx, result.AccountID, bindingKeys, now); err != nil {
			return err
		}
		// Tagged 0.438.3 commits the direct session after a successful
		// WebSocket turn even when a previous-response hard owner excluded it
		// from candidate selection. MergeVerified preserves an existing
		// different owner as a durable conflict marker, never silently
		// taking over that profile's verified session or Compact alias.
		if previousBound && result.Response.FirstEventCommitted &&
			validHardBindingComponent04383(keys.session) && bindingKeys.session == "" {
			if err := router.rememberVerifiedAccountBinding(ctx, result.AccountID,
				affinityKeys{session: keys.session}, now); err != nil {
				return err
			}
		}
		router.rememberAccountTurnState(
			ctx, keys.previous, result.AccountID, responseTurnStateHome(accounts, result.AccountID), keys.turn, now,
		)
		return nil
	}
	if result.Response.StatusCode == http.StatusSwitchingProtocols {
		keys.previous = result.Response.WebSocketResponseID
		if !validHardBindingComponent04383(keys.previous) {
			keys.previous = ""
		}
		if turnState := responseTurnStateValue(result.Response); turnState != "" {
			keys.turn = turnState
		}
		bindingKeys, err := router.responseBindingKeys(ctx, keys, selection, result.AccountID)
		if err != nil {
			return err
		}
		now := router.now()
		if err := router.rememberVerifiedAccountBinding(ctx, result.AccountID, bindingKeys, now); err != nil {
			return err
		}
		router.rememberAccountTurnState(
			ctx, keys.previous, result.AccountID, responseTurnStateHome(accounts, result.AccountID), keys.turn, now,
		)
		return nil
	}
	stream := strings.Contains(strings.ToLower(result.Response.Header.Get("Content-Type")), "text/event-stream")
	if selection.RouteKind == quotamodel.RouteKindResponses &&
		(stream || result.Response.RequestedStreaming &&
			strings.TrimSpace(result.Response.Header.Get("Content-Type")) == "" &&
			result.Response.Header.Get("Content-Encoding") == "") {
		// The tagged SSE tap retains the original requested previous ID
		// so a committed previous_response_not_found can clear that dead
		// chain as well as any response IDs observed in the SSE prelude.
		result.Response.RequestPreviousResponseID = keys.previous
		if validHardBindingComponent04383(keys.turn) {
			result.Response.RequestTurnStateOverride = keys.turn
		}
	}
	if result.Prefix == nil && !stream {
		prefix, _, err := inspectResponse(result.Response.Body, router.maxInspect)
		if err != nil {
			result.Response.Body.Close()
			return &proxymodel.Error{StatusCode: 502, Message: "upstream response failed before commitment"}
		}
		result.Prefix = prefix
	}
	bindingKeys, err := router.responseBindingKeys(ctx, keys, selection, result.AccountID)
	if err != nil {
		result.Response.Body.Close()
		return err
	}
	if err := router.rememberVerifiedAccountBinding(ctx, result.AccountID, bindingKeys, router.now()); err != nil {
		result.Response.Body.Close()
		return err
	}
	if selection.RouteKind == quotamodel.RouteKindResponses &&
		(keys.previous != "" || keys.turn != "") &&
		validHardBindingComponent04383(keys.session) && bindingKeys.session == "" {
		// Tagged Prodex remembers the successful *HTTP Responses* request's
		// session even when previous_response_id or turn_state supplied its
		// hard owner. Existing foreign ownership becomes a durable conflict.
		// MergeVerified creates a durable conflict instead of stealing an
		// existing different account's session. This must not run for failed
		// attempts, ordinary unowned sessions or the separate Compact route.
		if err := router.rememberVerifiedAccountBinding(ctx, result.AccountID,
			affinityKeys{session: keys.session}, router.now()); err != nil {
			result.Response.Body.Close()
			return err
		}
	}
	var observeErr error
	if stream && selection.RouteKind == quotamodel.RouteKindResponses &&
		!externalProviderKind(result.ProviderKind) {
		// Prodex seeds RuntimeSseTapState from response header, then the
		// effective outbound turn-state override, then SSE lookahead. The
		// already-inspected prefix IDs must inherit that request override.
		observeErr = router.observeSSEBindingsWithTurn04383(
			ctx, result.AccountID, result.Response.Header, result.Prefix,
			result.Response.RequestTurnStateOverride)
	} else {
		observeErr = router.Observe(ctx, result.AccountID, result.Response.Header,
			result.Prefix, stream, result.ProviderKind)
	}
	if err := observeErr; err != nil {
		result.Response.Body.Close()
		return err
	}
	if selection.RouteKind == quotamodel.RouteKindCompact {
		// Tagged 0.438.3 records distinct Compact-origin lineage only after
		// HTTP Compact commits successfully. These durable aliases are used
		// by later WebSocket messages without changing their quota route.
		compact := affinityKeys{}
		if validHardBindingComponent04383(keys.session) {
			// A successful Compact request records session lineage even if
			// selection was hard-pinned by a previous response and therefore
			// excluded its session from the outbound selection keys.
			if bindingKeys.session == "" {
				if err := router.rememberVerifiedAccountBinding(ctx, result.AccountID,
					affinityKeys{session: keys.session}, router.now()); err != nil {
					result.Response.Body.Close()
					return err
				}
			}
			compact.session = "__compact_session__:" + keys.session
		}
		if turn := responseTurnStateValue(result.Response); validHardBindingComponent04383(turn) {
			compact.turn = "__compact_turn_state__:" + turn
		}
		if compact.hasAffinity() {
			if err := router.rememberVerifiedAccountBinding(ctx, result.AccountID, compact, router.now()); err != nil {
				result.Response.Body.Close()
				return err
			}
		}
	}
	if promptCacheKey != "" {
		if len(result.Prefix) > 0 {
			if stream {
				decoder := sse.NewDecoder(int(router.maxInspect))
				for _, event := range decoder.Feed(result.Prefix) {
					router.observePromptCachePayload(result.AccountID, promptCacheKey, event)
				}
			} else {
				router.observePromptCachePayload(result.AccountID, promptCacheKey, result.Prefix)
			}
		}
		if stream {
			router.wrapPromptCacheSSEObservation(result.AccountID, promptCacheKey, result.Response)
		}
	}
	if stream {
		router.wrapResponsesStreamLatency(result.AccountID, selection, result.Response, len(result.Prefix) > 0)
	}
	return nil
}

func (router *Router) responseBindingKeys(
	ctx context.Context,
	keys affinityKeys,
	selection quotamodel.Selection,
	accountID string,
) (affinityKeys, error) {
	bindingKeys := affinityKeysForRoute(keys, selection.RouteKind)
	if selection.RouteKind == quotamodel.RouteKindCompact && bindingKeys.turn != "" && bindingKeys.session != "" {
		turnOwner, err := router.affinity.owner(ctx, affinityKeys{turn: bindingKeys.turn}, router.now())
		if err != nil {
			return affinityKeys{}, err
		}
		if turnOwner != "" && turnOwner != accountID {
			bindingKeys.session = ""
		}
	}
	return bindingKeys, nil
}

// Prodex 0.438.3's RuntimeHardBindingIdentity Mojo component classifier
// applies the same strict UTF-8, length and control rules to response,
// session and turn bindings as to Compact lineage aliases. Ordinary
// upstream transport headers remain byte-preserved independently.
func validHardBindingComponent04383(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r <= 31 || (r >= 127 && r <= 159) {
			return false
		}
	}
	return true
}
