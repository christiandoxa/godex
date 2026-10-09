package openai

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketAuth struct{}

func (websocketAuth) ReadAuth(context.Context, string) (proxymodel.Auth, error) {
	return proxymodel.Auth{AccessToken: "managed-token", AccountID: "managed-account"}, nil
}

func TestExecuteWebSocketReturnsDuplexBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Header.Get("Sec-WebSocket-Key")
		digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		accept := base64.StdEncoding.EncodeToString(digest[:])
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
		_ = buffered.Flush()
	}))
	defer upstream.Close()
	transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.ExecuteWebSocket(context.Background(), proxymodel.Request{
		Method: http.MethodGet,
		Path:   "/responses",
		Header: http.Header{
			"Connection":            {"Upgrade"},
			"Upgrade":               {"websocket"},
			"Sec-Websocket-Key":     {"dGhlIHNhbXBsZSBub25jZQ=="},
			"Sec-WebSocket-Version": {"13"},
		},
	}, proxymodel.Account{Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if _, ok := response.Body.(io.ReadWriteCloser); !ok {
		t.Fatalf("body type = %T", response.Body)
	}
	_ = response.Body.Close()
}

func TestExecuteWebSocketRejectsInvalidAccept(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = io.WriteString(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: invalid\r\n\r\n")
		_ = buffered.Flush()
	}))
	defer upstream.Close()
	transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	_, err = transport.ExecuteWebSocket(context.Background(), proxymodel.Request{
		Method: http.MethodGet,
		Path:   "/responses",
		Header: http.Header{
			"Connection":            {"Upgrade"},
			"Upgrade":               {"websocket"},
			"Sec-Websocket-Key":     {"dGhlIHNhbXBsZSBub25jZQ=="},
			"Sec-WebSocket-Version": {"13"},
		},
	}, proxymodel.Account{Home: "synthetic-home"})
	if err == nil {
		t.Fatal("invalid upstream accept was accepted")
	}
}

func TestWebSocketHTTPClientBoundsResponseHeaderWait(t *testing.T) {
	base := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second}}
	client := newWebSocketHTTPClient(base)
	defer client.CloseIdleConnections()
	transport := client.Transport.(*http.Transport)
	if transport.ResponseHeaderTimeout != websocketConnectTimeout {
		t.Fatalf("websocket response-header timeout = %s, want %s", transport.ResponseHeaderTimeout, websocketConnectTimeout)
	}
	if base.Transport.(*http.Transport).ResponseHeaderTimeout != 30*time.Second {
		t.Fatal("WebSocket timeout changed the shared HTTP transport")
	}
}

func TestWebSocketHandshakeContextCancelsAtTimeout(t *testing.T) {
	ctx, cancel, timer := websocketHandshakeContext(context.Background(), time.Millisecond)
	defer cancel()
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("WebSocket handshake context did not time out")
	}
	if ctx.Err() == nil {
		t.Fatal("timed-out WebSocket handshake context remained active")
	}
}

func TestExecuteWebSocketReturnsAfterUpstream101BeforeSocketCloses(t *testing.T) {
	handshakeSent := make(chan struct{})
	upstreamRead := make(chan byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Header.Get("Sec-WebSocket-Key")
		digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(digest[:]))
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush upstream handshake: %v", err)
			return
		}
		close(handshakeSent)
		_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
		value, err := buffered.Reader.ReadByte()
		if err == nil {
			upstreamRead <- value
		}
	}))
	defer upstream.Close()

	transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	headers := make(http.Header)
	headers.Set("Connection", "Upgrade")
	headers.Set("Upgrade", "websocket")
	headers.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	headers.Set("Sec-WebSocket-Version", "13")
	result := make(chan struct {
		response *proxymodel.Response
		err      error
	}, 1)
	go func() {
		response, executeErr := transport.ExecuteWebSocket(context.Background(), proxymodel.Request{
			Method: http.MethodGet, Path: "/backend-api/codex/responses", Header: headers,
		}, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
		result <- struct {
			response *proxymodel.Response
			err      error
		}{response: response, err: executeErr}
	}()

	select {
	case <-handshakeSent:
	case <-time.After(time.Second):
		t.Fatal("upstream did not flush websocket handshake")
	}
	select {
	case outcome := <-result:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.response == nil || outcome.response.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("websocket response = %#v", outcome.response)
		}
		duplex, ok := outcome.response.Body.(io.ReadWriteCloser)
		if !ok {
			t.Fatalf("websocket body type = %T", outcome.response.Body)
		}
		if _, err := duplex.Write([]byte{0x42}); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-upstreamRead:
			if got != 0x42 {
				t.Fatalf("upstream read byte = %#x", got)
			}
		case <-time.After(time.Second):
			t.Fatal("duplex websocket body did not reach upstream")
		}
		_ = duplex.Close()
	case <-time.After(time.Second):
		t.Fatal("ExecuteWebSocket blocked after upstream 101 was flushed")
	}
}

func TestExecuteWebSocketReturnsWithExtraUpstreamHandshakeHeaders(t *testing.T) {
	handshakeSent := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Header.Get("Sec-WebSocket-Key")
		digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\nSec-WebSocket-Protocol: codex.realtime\r\nSec-WebSocket-Extensions: permessage-deflate\r\nX-Upstream-Handshake: ready\r\n\r\n", base64.StdEncoding.EncodeToString(digest[:]))
		_ = buffered.Flush()
		close(handshakeSent)
		_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, _ = buffered.Reader.ReadByte()
	}))
	defer upstream.Close()
	transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	headers := make(http.Header)
	headers.Set("Connection", "Upgrade")
	headers.Set("Upgrade", "websocket")
	headers.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	headers.Set("Sec-WebSocket-Version", "13")
	result := make(chan error, 1)
	go func() {
		response, executeErr := transport.ExecuteWebSocket(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: "/backend-api/codex/responses", Header: headers}, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
		if executeErr == nil && response != nil {
			_ = response.Body.Close()
		}
		result <- executeErr
	}()
	<-handshakeSent
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("ExecuteWebSocket blocked on extra 101 handshake headers")
	}
}

func TestExecuteWebSocketDuplexBodyAllowsConcurrentReadAndWrite(t *testing.T) {
	handshakeSent := make(chan struct{})
	upstreamRead := make(chan byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Header.Get("Sec-WebSocket-Key")
		digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(digest[:]))
		_ = buffered.Flush()
		close(handshakeSent)
		_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
		value, err := buffered.Reader.ReadByte()
		if err == nil {
			upstreamRead <- value
		}
	}))
	defer upstream.Close()
	transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	headers := make(http.Header)
	headers.Set("Upgrade", "websocket")
	headers.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	response, err := transport.ExecuteWebSocket(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: "/backend-api/codex/responses", Header: headers}, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	duplex := response.Body.(io.ReadWriteCloser)
	defer duplex.Close()
	<-handshakeSent

	readStarted := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		close(readStarted)
		var one [1]byte
		_, readErr := duplex.Read(one[:])
		readDone <- readErr
	}()
	<-readStarted
	time.Sleep(25 * time.Millisecond)
	writeDone := make(chan error, 1)
	go func() {
		_, writeErr := duplex.Write([]byte{0x42})
		writeDone <- writeErr
	}()
	select {
	case writeErr := <-writeDone:
		if writeErr != nil {
			t.Fatal(writeErr)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("websocket duplex body write blocked behind concurrent read")
	}
	select {
	case got := <-upstreamRead:
		if got != 0x42 {
			t.Fatalf("upstream byte = %#x", got)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent websocket write did not reach upstream")
	}
	_ = duplex.Close()
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("blocked websocket read did not unblock on close")
	}
}
