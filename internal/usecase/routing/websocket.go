package routing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const maxWebSocketTurnStateBytes = 4 << 10

type websocketGateway interface {
	ExecuteWebSocket(context.Context, proxymodel.Request, proxymodel.Account) (*proxymodel.Response, error)
}

type websocketMessageSessionCloser interface {
	CloseWebSocketSession(uint64)
}

func (router *Router) executeWebSocketRequest(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	if request.WebSocketMessage {
		websocket, ok := router.gateway.(websocketMessageGateway)
		if !ok {
			return nil, errors.New("routing gateway does not support websocket messages")
		}
		return router.executeWebSocketMessage(ctx, request, account, websocket)
	}
	websocket, ok := router.gateway.(websocketGateway)
	if !ok {
		return nil, errors.New("routing gateway does not support websocket upgrades")
	}
	return router.executeWebSocket(ctx, request, account, websocket)
}

func (router *Router) executeWebSocket(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	websocket websocketGateway,
) (*proxymodel.Response, error) {
	for reload := 0; reload < 2; reload++ {
		response, err := router.observedWebSocketConnectAttempt(ctx, request, account, func() (*proxymodel.Response, error) {
			return websocket.ExecuteWebSocket(ctx, request, account)
		})
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusUnauthorized || reload == 1 {
			return response, nil
		}
		_ = response.Body.Close()
	}
	return nil, errors.New("websocket authentication retry failed")
}

func (router *Router) executeWebSocketMessage(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	websocket websocketMessageGateway,
) (*proxymodel.Response, error) {
	ownerRetryUsed, authReloadUsed := false, false
	turnStateRetryDelays := [...]time.Duration{75 * time.Millisecond, 200 * time.Millisecond, 500 * time.Millisecond}
	turnStateRetryIndex := 0
	for {
		response, err := router.observedWebSocketMessageAttempt(ctx, request, account, func() (*proxymodel.Response, error) {
			return websocket.ExecuteWebSocketMessage(ctx, request, account)
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errors.New("routing gateway returned no websocket message response")
		}
		if request.FirstEventRetryUsed {
			response.FirstEventRetryUsed = true
		}
		if response.StatusCode == http.StatusUnauthorized && !authReloadUsed {
			_ = response.Body.Close()
			authReloadUsed = true
			continue
		}
		if !ownerRetryUsed && websocketOwnerTransportRetry(response, request) {
			_ = response.Body.Close()
			ownerRetryUsed = true
			continue
		}
		turnState := ""
		if turnStateRetryIndex < len(turnStateRetryDelays) {
			turnState = websocketTurnStateRetry(response, request)
		}
		if turnState != "" {
			_ = response.Body.Close()
			if err := router.wait(ctx, turnStateRetryDelays[turnStateRetryIndex]); err != nil {
				return nil, err
			}
			request = websocketTurnStateRequest(request, turnState)
			turnStateRetryIndex++
			continue
		}
		markStaleWebSocketContinuation(response, request)
		return response, nil
	}
}

func websocketOwnerTransportRetry(response *proxymodel.Response, request proxymodel.Request) bool {
	return response != nil && !response.FirstEventRetryUsed && !response.FirstEventCommitted &&
		!request.FirstEventRetryUsed && request.WebSocketMessage &&
		requestAffinity(request, request.Body).previous != "" &&
		response.PrecommitFailure != nil && response.PrecommitFailure.Transport &&
		strings.TrimSpace(response.WebSocketTurnState) != "" &&
		len(response.WebSocketTurnState) <= maxWebSocketTurnStateBytes
}

func websocketTurnStateRequest(
	request proxymodel.Request,
	turnState string,
) proxymodel.Request {
	request.Header = request.Header.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set("x-codex-turn-state", turnState)
	request.FirstEventRetryUsed = true
	return request
}

func websocketTurnStateRetry(response *proxymodel.Response, request proxymodel.Request) string {
	if response == nil || response.FirstEventCommitted || requestAffinity(request, request.Body).previous == "" ||
		response.PrecommitFailure == nil ||
		!strings.EqualFold(strings.TrimSpace(response.PrecommitFailure.Code), "previous_response_not_found") {
		return ""
	}
	turnState := strings.TrimSpace(response.WebSocketTurnState)
	if len(turnState) > maxWebSocketTurnStateBytes {
		return ""
	}
	return turnState
}

func markStaleWebSocketContinuation(response *proxymodel.Response, request proxymodel.Request) {
	if response == nil || response.PrecommitFailure == nil ||
		!strings.EqualFold(strings.TrimSpace(response.PrecommitFailure.Code), "previous_response_not_found") {
		return
	}
	if requestAffinity(request, request.Body).previous != "" {
		response.PrecommitFailure.StaleContinuation = true
	}
}

func (router *Router) CloseWebSocketSession(sessionID uint64) {
	if gateway, ok := router.gateway.(websocketMessageSessionCloser); ok {
		gateway.CloseWebSocketSession(sessionID)
	}
}
