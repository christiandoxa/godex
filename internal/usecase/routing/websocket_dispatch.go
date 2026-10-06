package routing

import (
	"context"
	"errors"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketMessageGateway interface {
	ExecuteWebSocketMessage(
		context.Context,
		proxymodel.Request,
		proxymodel.Account,
	) (*proxymodel.Response, error)
}

func requestRoutingAffinity(request proxymodel.Request) affinityKeys {
	keys := requestAffinity(request, request.Body)
	if !request.WebSocketMessage {
		return keys
	}
	metadata := parseWebSocketRequestMetadata(request)
	if metadata.previousResponseID != "" {
		keys.previous = metadata.previousResponseID
	}
	if metadata.sessionID != "" {
		keys.session = metadata.sessionID
	}
	if metadata.turnState != "" {
		keys.turn = metadata.turnState
	}
	return keys
}

func (router *Router) executeRouted(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	hardAffinity bool,
) (*proxymodel.Response, error) {
	if !request.WebSocketMessage {
		return router.execute(ctx, request, account)
	}
	websocket, ok := router.gateway.(websocketMessageGateway)
	if !ok {
		return nil, errors.New("routing gateway does not support websocket messages")
	}

	metadata := parseWebSocketRequestMetadata(request)
	routingAffinity := requestRoutingAffinity(request)
	if metadata.previousResponseID == "" {
		metadata.previousResponseID = routingAffinity.previous
	}
	if metadata.sessionID == "" {
		metadata.sessionID = routingAffinity.session
	}
	if metadata.turnState == "" {
		metadata.turnState = routingAffinity.turn
	}
	policy := request.WebSocketPolicy
	policy.PromoteCommittedProfile =
		!hardAffinity && metadata.previousResponseID == "" && metadata.turnState == ""
	policy.RequestPreviousResponse = metadata.previousResponseID != ""
	policy.RequestSession = metadata.sessionID != ""
	policy.RequestTurnState = metadata.turnState != ""
	request.WebSocketPolicy = policy

	trustedPreviousAffinity, err := router.websocketTrustedPreviousAffinity(
		ctx, metadata.previousResponseID, account.ID,
	)
	if err != nil {
		return nil, err
	}
	authReloadUsed := false
	ownerTransportRetryUsed := false
	previousRetryIndex := 0
	current := request
	for {
		response, err := router.observedWebSocketMessageAttempt(ctx, current, account, func() (*proxymodel.Response, error) {
			return websocket.ExecuteWebSocketMessage(ctx, current, account)
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errors.New("routing gateway returned no websocket message response")
		}
		if response.StatusCode == http.StatusUnauthorized && !authReloadUsed {
			closeWebSocketRoutingResponse(response)
			authReloadUsed = true
			continue
		}
		if !ownerTransportRetryUsed && hardAffinity &&
			websocketOwnerTransportRecovery(response, metadata.previousResponseID) {
			closeWebSocketRoutingResponse(response)
			ownerTransportRetryUsed = true
			continue
		}
		if !websocketPreviousResponseNotFound(response, metadata.previousResponseID) {
			return response, nil
		}
		if response.PrecommitFailure.InvalidPreviousResponseID {
			return router.handleInvalidWebSocketPreviousResponse(
				ctx, response, metadata, account, trustedPreviousAffinity,
			)
		}

		turnState := strings.TrimSpace(response.WebSocketTurnState)
		plan := planWebSocketPreviousResponse(websocketPreviousResponsePlanInput{
			previousPresent:                 metadata.previousResponseID != "",
			hasTurnStateRetry:               turnState != "",
			requestRequiresPreviousAffinity: metadata.requiresPreviousResponseAffinity,
			trustedPreviousAffinity:         trustedPreviousAffinity,
			requestTurnStatePresent:         metadata.turnState != "",
			retryIndex:                      previousRetryIndex,
		})
		if plan.retryOwner {
			closeWebSocketRoutingResponse(response)
			if err := router.wait(ctx, plan.retryDelay); err != nil {
				return nil, err
			}
			previousRetryIndex++
			if turnState != "" {
				current = websocketTurnStateOverride(current, turnState)
			}
			continue
		}
		if plan.staleContinuation {
			return staleWebSocketContinuationResponse(response), nil
		}
		return response, nil
	}
}

func (router *Router) websocketTrustedPreviousAffinity(
	ctx context.Context,
	previousResponseID, accountID string,
) (bool, error) {
	if strings.TrimSpace(previousResponseID) == "" {
		return false, nil
	}
	owner, err := router.affinity.owner(
		ctx,
		affinityKeys{previous: previousResponseID},
		router.now(),
	)
	if err != nil {
		return false, err
	}
	return owner != "" && owner == accountID, nil
}

func websocketPreviousResponseNotFound(
	response *proxymodel.Response,
	previousResponseID string,
) bool {
	return response != nil &&
		strings.TrimSpace(previousResponseID) != "" &&
		!response.FirstEventCommitted &&
		response.PrecommitFailure != nil &&
		strings.EqualFold(
			strings.TrimSpace(response.PrecommitFailure.Code),
			"previous_response_not_found",
		)
}

func websocketOwnerTransportRecovery(response *proxymodel.Response, previousResponseID string) bool {
	return response != nil &&
		strings.TrimSpace(previousResponseID) != "" &&
		!response.FirstEventCommitted &&
		response.PrecommitFailure != nil &&
		response.PrecommitFailure.Transport &&
		strings.TrimSpace(response.WebSocketTurnState) != ""
}

func websocketTurnStateOverride(
	request proxymodel.Request,
	turnState string,
) proxymodel.Request {
	request.Header = request.Header.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set("x-codex-turn-state", turnState)
	request.FirstEventRetryUsed = true
	request.WebSocketPolicy.TurnStateOverride = true
	return request
}

func closeWebSocketRoutingResponse(response *proxymodel.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func websocketCommittedAffinity(response *proxymodel.Response) affinityKeys {
	if response == nil || !response.WebSocketFrames || !response.FirstEventCommitted {
		return affinityKeys{}
	}
	return affinityKeys{
		previous: response.WebSocketResponseID,
		turn:     response.WebSocketTurnState,
	}
}
