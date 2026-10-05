package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (proxy *Proxy) forwardWebSocketMessages(
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
	tunnel := &websocketTunnel{
		client: client, toClient: &websocketFrameWriter{writer: buffered.Writer}, cancel: cancel,
	}
	remove := proxy.trackWebSocketTunnel(tunnel)
	defer remove()
	defer proxy.router.CloseWebSocketSession(sessionID)
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
		messageID := proxy.sequence.Add(1)
		forwarded, err := proxy.router.Forward(sessionContext, proxymodel.Request{
			RequestID:          messageID,
			WebSocketSessionID: sessionID,
			Method:             request.Method, Path: request.URL.Path, RawPath: request.URL.EscapedPath(),
			RawQuery: request.URL.RawQuery, Header: request.Header.Clone(), Body: payload,
			QuotaSelection: quotaSelection(request.URL.Path, true, payload), WebSocketMessage: true,
		})
		if err != nil {
			if sessionContext.Err() != nil {
				return sessionContext.Err()
			}
			status, code, message := websocketRouteError(err)
			if writeErr := tunnel.toClient.writeText(websocketErrorPayload(status, code, message)); writeErr != nil {
				return writeErr
			}
			continue
		}
		activity.upstream(forwarded.Result.AccountID, forwarded.Result.Response.StatusCode)
		if forwarded.Result.Failed {
			activity.accountID = ""
		}
		response := forwarded.Result.Response
		tunnel.setUpstream(response.Body)
		if response.WebSocketFrames {
			err = copyWebSocketFrames(response.Body, tunnel.toClient)
		} else {
			err = proxy.forwardWebSocketHTTPError(response, forwarded.Result.Prefix, tunnel.toClient)
		}
		_ = forwarded.Close()
		tunnel.clearUpstream()
		if err != nil {
			return err
		}
	}
}
