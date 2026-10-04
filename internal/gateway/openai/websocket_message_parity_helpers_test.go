package openai

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func newWebSocketMessageParityServer(
	t *testing.T,
	headers http.Header,
	handle func(net.Conn, *bufio.Reader, *http.Request),
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Header.Get("Sec-WebSocket-Key")
		digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = fmt.Fprintf(
			buffered,
			"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n",
			base64.StdEncoding.EncodeToString(digest[:]),
		)
		for name, values := range headers {
			for _, value := range values {
				_, _ = fmt.Fprintf(buffered, "%s: %s\r\n", name, value)
			}
		}
		_, _ = io.WriteString(buffered, "\r\n")
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush upstream handshake: %v", err)
			return
		}
		handle(connection, buffered.Reader, request)
	}))
}

func websocketParityRequest(body string, sessionID uint64, policy proxymodel.WebSocketPolicy) proxymodel.Request {
	return proxymodel.Request{
		Method: http.MethodGet,
		Path:   "/backend-api/codex/responses",
		Header: http.Header{
			"Connection":            {"Upgrade"},
			"Upgrade":               {"websocket"},
			"Sec-WebSocket-Key":     {"dGhlIHNhbXBsZSBub25jZQ=="},
			"Sec-WebSocket-Version": {"13"},
		},
		Body:               []byte(body),
		WebSocketMessage:   true,
		WebSocketSessionID: sessionID,
		WebSocketPolicy:    policy,
	}
}

func readWebSocketParityRequest(reader *bufio.Reader) ([]byte, error) {
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		return nil, err
	}
	payload, err := frame.ReadPayload(reader, 1<<20)
	if err != nil {
		return nil, err
	}
	frame.Unmask(payload)
	return payload, nil
}

func writeWebSocketParityEvent(writer io.Writer, payload []byte) error {
	return websocketframe.WriteFrame(writer, 1, payload, false)
}

func websocketParityFrames(t *testing.T, payloads ...[]byte) []byte {
	t.Helper()
	var frames bytes.Buffer
	for _, payload := range payloads {
		if err := writeWebSocketParityEvent(&frames, payload); err != nil {
			t.Fatal(err)
		}
	}
	return frames.Bytes()
}

func newWebSocketParityTransport(t *testing.T, server *httptest.Server) *Transport {
	t.Helper()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transport.Close)
	return transport
}
