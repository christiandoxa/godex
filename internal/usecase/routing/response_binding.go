package routing

import (
	"context"
	"net/http"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/sse"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) bindSuccessfulResponse(
	ctx context.Context,
	result *proxymodel.Forwarded,
	accounts []proxymodel.Account,
	keys affinityKeys,
	websocketMessage bool,
	selection quotamodel.Selection,
	promptCacheKey string,
) error {
	if result.Response.PrecommitFailure != nil && result.Response.PrecommitFailure.StaleContinuation {
		result.Failed = true
		return nil
	}
	if result.Failed || result.Response.StatusCode >= 400 {
		return nil
	}
	if keys.turn != "" && router.affinity.deadTurnStateContext(ctx, keys.turn, router.now()) {
		keys.turn = ""
	}
	router.clearPreviousResponseFailures(ctx, result.AccountID, keys.previous)
	router.rememberPromptCacheOwner(result.AccountID, promptCacheKey, router.now())
	if websocketMessage {
		if result.Response.WebSocketResponseID != "" {
			keys.previous = result.Response.WebSocketResponseID
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
		router.rememberAccountTurnState(
			ctx, keys.previous, result.AccountID, responseTurnStateHome(accounts, result.AccountID), keys.turn, now,
		)
		return nil
	}
	if result.Response.StatusCode == http.StatusSwitchingProtocols {
		keys.previous = result.Response.WebSocketResponseID
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
	if err := router.Observe(ctx, result.AccountID, result.Response.Header, result.Prefix, stream, result.ProviderKind); err != nil {
		result.Response.Body.Close()
		return err
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
