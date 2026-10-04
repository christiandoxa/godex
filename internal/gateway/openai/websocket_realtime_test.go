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

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestExecuteWebSocketMessageRealtimeCommitsAfterFirstTextWithoutUpstreamOutput(t *testing.T) {
	received := make(chan string, 1)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := request.Header.Get("Sec-WebSocket-Key")
		digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(
			buffered,
			"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
			base64.StdEncoding.EncodeToString(digest[:]),
		)
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush handshake: %v", err)
			return
		}
		frame, err := websocketframe.ReadHeader(buffered.Reader)
		if err != nil {
			t.Errorf("read first realtime frame: %v", err)
			return
		}
		payload, err := frame.ReadPayload(buffered.Reader, 1<<20)
		if err != nil {
			t.Errorf("read first realtime payload: %v", err)
			return
		}
		frame.Unmask(payload)
		received <- string(payload)
		<-release
	}))
	defer upstream.Close()

	transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := transport.ExecuteWebSocketMessage(ctx, proxymodel.Request{
		Method:           http.MethodGet,
		Path:             "/backend-api/codex/live",
		Header:           make(http.Header),
		Body:             []byte(`{"type":"session.update"}`),
		WebSocketMessage: true,
		WebSocketPolicy:  proxymodel.WebSocketPolicy{RealtimeDuplex: true},
	}, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols ||
		!response.FirstEventCommitted ||
		!response.WebSocketRealtimeDuplex ||
		!response.WebSocketFrames {
		_ = response.Body.Close()
		t.Fatalf("realtime response = %#v", response)
	}
	if _, ok := response.Body.(io.ReadWriteCloser); !ok {
		_ = response.Body.Close()
		t.Fatalf("realtime body type = %T", response.Body)
	}
	select {
	case payload := <-received:
		if payload != `{"type":"session.update"}` {
			_ = response.Body.Close()
			t.Fatalf("first upstream payload = %q", payload)
		}
	case <-time.After(time.Second):
		_ = response.Body.Close()
		t.Fatal("first realtime message did not reach upstream")
	}
	_ = response.Body.Close()
	close(release)
}
