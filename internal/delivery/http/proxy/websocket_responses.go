package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (proxy *Proxy) forwardResponsesWebSocket(
	writer http.ResponseWriter,
	request *http.Request,
	activity *requestActivity,
	lifecycle *requestLifecycle,
	clientKey string,
) error {
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		return http.ErrNotSupported
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		return err
	}

	sessionID := proxy.sequence.Add(1)
	sessionContext, cancel := context.WithCancel(request.Context())
	tunnel := &responsesWebSocketTunnel{
		client:   client,
		toClient: &websocketFrameWriter{writer: buffered.Writer},
		cancel:   cancel,
	}
	remove := proxy.trackResponsesWebSocketTunnel(tunnel)
	defer remove()
	defer proxy.router.ReleaseWebSocketMessageSession(sessionID)
	defer tunnel.close()

	lifecycle.commit()
	handshake := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + websocketAcceptKey(clientKey) + "\r\n\r\n"
	if _, err := io.WriteString(buffered.Writer, handshake); err != nil {
		return err
	}
	if err := buffered.Writer.Flush(); err != nil {
		return err
	}

	for {
		payload, kind, err := readWebSocketClientMessage(buffered.Reader, tunnel.toClient)
		if err != nil {
			if errors.Is(err, io.EOF) {
				lifecycle.complete()
				return nil
			}
			return err
		}
		switch kind {
		case websocketInputClose:
			lifecycle.complete()
			return nil
		case websocketInputBinary:
			if err := tunnel.toClient.writeText([]byte(websocketBinaryMessageError)); err != nil {
				return err
			}
			continue
		}
		if isResponseProcessedMessage(payload) {
			continue
		}

		if proxy.redactor != nil && len(payload) > 0 {
			redacted, err := proxy.redactor.Redact(sessionContext, payload)
			if err != nil {
				if writeErr := tunnel.toClient.writeText(
					responsesWebSocketErrorPayload(http.StatusBadGateway, "presidio_redaction_failed", "gateway PII redaction failed"),
				); writeErr != nil {
					return writeErr
				}
				continue
			}
			payload = redacted
		}
		smart := prepareSmartContextWebSocketBody(
			proxy.smartContextEnabled, request.URL.Path, request.Header, payload,
		)
		payload = smart.Body

		forwarded, err := proxy.router.Forward(sessionContext, proxymodel.Request{
			RequestID:          proxy.sequence.Add(1),
			Method:             request.Method,
			Path:               request.URL.Path,
			RawPath:            request.URL.EscapedPath(),
			RawQuery:           request.URL.RawQuery,
			Header:             request.Header.Clone(),
			Body:               payload,
			WebSocketMessage:   true,
			WebSocketSessionID: sessionID,
			WebSocketPolicy: proxymodel.WebSocketPolicy{
				RealtimeDuplex: websocketRealtimeDuplexPath(request.URL.Path),
			},
		})
		if err != nil {
			if sessionContext.Err() != nil {
				return sessionContext.Err()
			}
			status, code, message := responsesWebSocketRouteError(err)
			if writeErr := tunnel.toClient.writeText(
				responsesWebSocketErrorPayload(status, code, message),
			); writeErr != nil {
				return writeErr
			}
			continue
		}

		result := forwarded.Result
		if result.Response == nil {
			_ = forwarded.Close()
			return errors.New("websocket routing returned no response")
		}
		activity.upstream(result.AccountID, result.Response.StatusCode)
		if result.Failed {
			activity.accountID = "<redacted>"
		}
		tunnel.setUpstream(result.Response.Body)
		if result.Response.WebSocketRealtimeDuplex {
			duplex, ok := result.Response.Body.(io.ReadWriteCloser)
			if !ok {
				_ = forwarded.Close()
				return errors.New("realtime websocket response is not duplex")
			}
			err = runRealtimeWebSocketDuplex(
				buffered.Reader, tunnel.toClient, duplex, tunnel.close,
			)
			closeErr := forwarded.Close()
			tunnel.clearUpstream()
			if err == nil {
				lifecycle.complete()
			}
			if err != nil {
				return err
			}
			return closeErr
		}
		if result.Response.WebSocketFrames {
			err = copyWebSocketFrames(result.Response.Body, tunnel.toClient)
		} else {
			err = proxy.forwardResponsesWebSocketHTTPBody(
				result.Response, result.Prefix, tunnel.toClient,
			)
		}
		closeErr := forwarded.Close()
		tunnel.clearUpstream()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
