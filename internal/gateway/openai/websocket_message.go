package openai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (transport *Transport) ExecuteWebSocketMessage(
	ctx context.Context,
	input proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	turnStateOverride := strings.TrimSpace(input.Header.Get("x-codex-turn-state"))
	session := transport.takeWebSocketMessageSession(input.WebSocketSessionID, account, turnStateOverride)
	connection := session.connection
	state := &websocketMessageState{turnState: session.turnState}
	reusedSession := connection != nil
	reuseIdle := time.Duration(0)
	connectDuration := time.Duration(0)
	if reusedSession {
		reuseIdle = time.Since(session.completed)
	}
	if connection == nil {
		handshake := input
		handshake.Body = nil
		connectStarted := time.Now()
		response, err := transport.ExecuteWebSocket(ctx, handshake, account)
		connectDuration = time.Since(connectStarted)
		if err != nil {
			return nil, err
		}
		response.FirstEventRetryUsed = input.FirstEventRetryUsed
		state.turnState = strings.TrimSpace(response.Header.Get("x-codex-turn-state"))
		response.WebSocketTurnState = state.turnState
		if response.StatusCode != http.StatusSwitchingProtocols {
			return response, nil
		}
		var ok bool
		connection, ok = response.Body.(io.ReadWriteCloser)
		if !ok {
			_ = response.Body.Close()
			return nil, errors.New("upstream websocket connection is not duplex")
		}
	}
	if input.WebSocketPolicy.RealtimeDuplex {
		if err := writeWebSocketTextFrame(connection, input.Body); err != nil {
			_ = connection.Close()
			return nil, fmt.Errorf("send upstream realtime websocket message: %w", err)
		}
		response := websocketRealtimeCommittedResponse(
			connection, state.turnState, input.FirstEventRetryUsed, reusedSession, reuseIdle,
		)
		response.UpstreamConnectDuration = connectDuration
		return response, nil
	}
	watchdog := newWebSocketReadWatchdog(connection, websocketPrecommitProgressTimeout)
	connection = watchdog
	// The idle timer cannot abort a blocked frame read when the logical
	// request is cancelled. Disarm this hook before returning a committed
	// stream so a later precommit deadline cannot close its connection.
	stopCancellation := context.AfterFunc(ctx, func() { _ = watchdog.Close() })
	defer stopCancellation()
	if err := ctx.Err(); err != nil {
		_ = watchdog.Close()
		return nil, err
	}
	if err := writeWebSocketTextFrame(connection, input.Body); err != nil {
		_ = connection.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if reusedSession {
			return websocketTransportFailure(
				"", state.turnState, input.FirstEventRetryUsed, true, reuseIdle,
			), nil
		}
		return nil, fmt.Errorf("send upstream websocket message: %w", err)
	}
	plan := websocketResponsePlanFor(input, reusedSession)
	response, err := transport.readWebSocketMessage(
		ctx, input, account, connection, state, reusedSession, reuseIdle, plan,
	)
	if response != nil && response.UpstreamConnectDuration == 0 {
		response.UpstreamConnectDuration = connectDuration
	}
	return response, err
}

func websocketRealtimeCommittedResponse(
	connection io.ReadWriteCloser,
	turnState string,
	retryUsed bool,
	reusedSession bool,
	reuseIdle time.Duration,
) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode:              http.StatusSwitchingProtocols,
		Header:                  make(http.Header),
		Body:                    connection,
		WebSocketTurnState:      turnState,
		WebSocketFrames:         true,
		WebSocketReusedSession:  reusedSession,
		WebSocketReuseIdle:      reuseIdle,
		WebSocketRealtimeDuplex: true,
		FirstEventRetryUsed:     retryUsed,
		FirstEventCommitted:     true,
	}
}

func writeWebSocketTextFrame(connection io.Writer, payload []byte) error {
	return websocketframe.WriteFrame(connection, 1, payload, true)
}

func (transport *Transport) readWebSocketMessage(
	ctx context.Context,
	input proxymodel.Request,
	account proxymodel.Account,
	connection io.ReadWriteCloser,
	state *websocketMessageState,
	reusedSession bool,
	reuseIdle time.Duration,
	plan websocketResponsePlan,
) (*proxymodel.Response, error) {
	buffered := make([]byte, 0, websocketPrecommitLookaheadBytes)
	responseID := ""
	holdBytes := 0
	for {
		if err := ctx.Err(); err != nil {
			_ = connection.Close()
			return nil, err
		}
		event, readErr := readWebSocketEvent(connection)
		if readErr != nil {
			if ctx.Err() != nil {
				_ = connection.Close()
				return nil, ctx.Err()
			}
			if reusedSession || plan.transportRetryAllowed {
				_ = connection.Close()
				return websocketTransportFailure(
					responseID, state.turnState, input.FirstEventRetryUsed, reusedSession, reuseIdle,
				), nil
			}
			_ = connection.Close()
			return nil, fmt.Errorf("runtime websocket upstream failed before response commitment: %w", readErr)
		}
		if err := ctx.Err(); err != nil {
			_ = connection.Close()
			return nil, err
		}
		// Hidden keepalive, metadata and prelude frames are still precommit.
		// Do not promote their socket idle timer to the 300-second committed
		// stream timer: Prodex 0.438.2 retains the original request watchdog
		// until actual model output has passed the precommit checks below.
		if event.turnState != "" {
			state.turnState = event.turnState
		}
		if responseID == "" && event.responseID != "" {
			responseID = event.responseID
		}
		if isWebSocketPrecommitHold(event.kind) {
			holdBytes += len(event.payload)
			if !plan.holdPromotionAllowed && holdBytes > websocketPrecommitHardAffinityBytes {
				_ = connection.Close()
				return nil, errors.New("runtime websocket precommit hold exceeded its bounded hard-affinity limit")
			}
			buffered = append(buffered, event.frames...)
			if plan.holdPromotionAllowed && holdBytes >= websocketPrecommitLookaheadBytes {
				return websocketCommittedResponse(
					connection, buffered, "", responseID, state,
					input.FirstEventRetryUsed, reusedSession, reuseIdle,
					transport.websocketMessageRecycle(input.WebSocketSessionID, account),
				), nil
			}
			continue
		}
		if event.retryCode != "" {
			if response, retried, err := transport.retryWebSocketConnectionLimit(
				ctx, input, account, connection, event, state.turnState, reusedSession,
			); retried || err != nil {
				return response, err
			}
			_ = connection.Close()
			return websocketPrecommitFailure(
				event.frames, responseID, state.turnState, input.FirstEventRetryUsed,
				event, reusedSession, reuseIdle,
			), nil
		}

		buffered = append(buffered, event.frames...)
		if watchdog, ok := connection.(*websocketReadWatchdog); ok {
			watchdog.markCommittedIdle04382(transport.CommittedWebSocketIdleTimeout04382())
		}
		return websocketCommittedResponse(
			connection, buffered, terminalKind(event), responseID, state,
			input.FirstEventRetryUsed, reusedSession, reuseIdle,
			transport.websocketMessageRecycle(input.WebSocketSessionID, account),
		), nil
	}
}

func terminalKind(event websocketEvent) string {
	if event.terminal {
		return event.kind
	}
	return ""
}

func (transport *Transport) websocketMessageRecycle(
	sessionID uint64,
	account proxymodel.Account,
) func(io.ReadWriteCloser, string) {
	return func(connection io.ReadWriteCloser, turnState string) {
		if watchdog, ok := connection.(*websocketReadWatchdog); ok {
			watchdog.pause()
		}
		transport.recycleWebSocketMessageSession(sessionID, account, connection, turnState)
	}
}

func (transport *Transport) retryWebSocketConnectionLimit(
	ctx context.Context,
	input proxymodel.Request,
	account proxymodel.Account,
	connection io.ReadWriteCloser,
	event websocketEvent,
	turnState string,
	reusedSession bool,
) (*proxymodel.Response, bool, error) {
	// Prodex 0.438.0 retries a connection-limit frame on the first fresh
	// connection as well as on a recycled one. This helper is reached only
	// before a frame commits output; FirstEventRetryUsed caps the retry at
	// exactly one and preserves the original account and opaque turn body.
	if input.FirstEventRetryUsed || !isWebSocketConnectionLimit(event.payload) {
		return nil, false, nil
	}
	_ = connection.Close()
	if turnState != "" {
		input.Header = input.Header.Clone()
		if input.Header == nil {
			input.Header = make(http.Header)
		}
		input.Header.Set("x-codex-turn-state", turnState)
		input.WebSocketPolicy.TurnStateOverride = true
	}
	input.FirstEventRetryUsed = true
	response, err := transport.ExecuteWebSocketMessage(ctx, input, account)
	return response, true, err
}

func websocketPrecommitFailure(
	frames []byte,
	responseID, turnState string,
	retryUsed bool,
	event websocketEvent,
	reusedSession bool,
	reuseIdle time.Duration,
) *proxymodel.Response {
	// Preserve bounded precommit error metadata for the use-case-owned
	// Retry-After planner. Committed frames never enter this path.
	var retryAdvice []byte
	if event.text && len(event.payload) <= 64<<10 {
		retryAdvice = append([]byte(nil), event.payload...)
	}
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body:                   io.NopCloser(bytes.NewReader(frames)),
		WebSocketResponseID:    responseID,
		WebSocketTurnState:     turnState,
		WebSocketFrames:        true,
		WebSocketReusedSession: reusedSession,
		WebSocketReuseIdle:     reuseIdle,
		FirstEventRetryUsed:    retryUsed,
		PrecommitFailure: &proxymodel.PrecommitFailure{
			Code: event.retryCode, InvalidPreviousResponseID: event.invalidPreviousResponseID,
			RetryAdviceJSON: retryAdvice,
		},
	}
}

func websocketCommittedResponse(
	connection io.ReadWriteCloser,
	frames []byte,
	terminalKind string,
	responseID string,
	state *websocketMessageState,
	retryUsed bool,
	reusedSession bool,
	reuseIdle time.Duration,
	recycle func(io.ReadWriteCloser, string),
) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body:                   newWebSocketResponseBody(connection, frames, terminalKind, state, recycle),
		WebSocketResponseID:    responseID,
		WebSocketTurnState:     state.turnState,
		WebSocketFrames:        true,
		WebSocketReusedSession: reusedSession,
		WebSocketReuseIdle:     reuseIdle,
		FirstEventRetryUsed:    retryUsed,
		FirstEventCommitted:    true,
	}
}

func websocketTransportFailure(
	responseID, turnState string,
	retryUsed, reusedSession bool,
	reuseIdle time.Duration,
) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode: http.StatusBadGateway, Header: make(http.Header),
		Body:                   io.NopCloser(bytes.NewReader(nil)),
		WebSocketResponseID:    responseID,
		WebSocketTurnState:     turnState,
		WebSocketFrames:        true,
		WebSocketReusedSession: reusedSession,
		WebSocketReuseIdle:     reuseIdle,
		FirstEventRetryUsed:    retryUsed,
		PrecommitFailure:       &proxymodel.PrecommitFailure{Transport: true},
	}
}
