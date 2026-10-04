package routing

import (
	"context"
	"errors"
	"net/http"

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
	keys.previous = metadata.previousResponseID
	keys.session = metadata.sessionID
	keys.turn = metadata.turnState
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
	policy := request.WebSocketPolicy
	policy.PromoteCommittedProfile =
		!hardAffinity && metadata.previousResponseID == "" && metadata.turnState == ""
	policy.RequestPreviousResponse = metadata.previousResponseID != ""
	policy.RequestSession = metadata.sessionID != ""
	policy.RequestTurnState = metadata.turnState != ""
	request.WebSocketPolicy = policy

	for reload := 0; reload < 2; reload++ {
		response, err := websocket.ExecuteWebSocketMessage(ctx, request, account)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized || reload == 1 {
			return response, nil
		}
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}
	return nil, errors.New("websocket authentication retry failed")
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
