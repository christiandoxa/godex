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

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestExecuteWebSocketUsesIndependentProdexUpstreamHandshake(t *testing.T) {
	const clientKey = "dGhlIHNhbXBsZSBub25jZQ=="
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamKey := request.Header.Get("Sec-WebSocket-Key")
		decoded, err := base64.StdEncoding.DecodeString(upstreamKey)
		if err != nil || len(decoded) != 16 {
			t.Errorf("upstream websocket key = %q, err=%v, decoded=%d", upstreamKey, err, len(decoded))
		}
		if upstreamKey == clientKey {
			t.Errorf("upstream websocket reused downstream client key %q", upstreamKey)
		}
		if request.Header.Get("Sec-WebSocket-Version") != "13" || request.Header.Get("Sec-WebSocket-Protocol") != "" || request.Header.Get("Sec-WebSocket-Extensions") != "" {
			t.Errorf("upstream websocket transport headers = %#v", request.Header)
		}
		if request.Header.Get("Origin") != "https://client.example" {
			t.Errorf("Origin = %q", request.Header.Get("Origin"))
		}
		if authorization := request.Header.Get("Authorization"); authorization == "" || authorization == "Bearer caller-secret" {
			t.Errorf("Authorization was not replaced: %q", authorization)
		}
		if account := request.Header.Get("ChatGPT-Account-Id"); account == "" || account == "caller-account" {
			t.Errorf("ChatGPT account was not replaced: %q", account)
		}
		digest := sha1.Sum([]byte(upstreamKey + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(digest[:]))
		_ = buffered.Flush()
	}))
	defer upstream.Close()

	transport, err := NewTransport(upstream.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	headers := make(http.Header)
	headers.Set("Connection", "keep-alive, Upgrade")
	headers.Set("Upgrade", "websocket")
	headers.Set("Sec-WebSocket-Key", clientKey)
	headers.Set("Sec-WebSocket-Version", "13")
	headers.Set("Sec-WebSocket-Protocol", "codex.realtime")
	headers.Set("Sec-WebSocket-Extensions", "permessage-deflate")
	headers.Set("Origin", "https://client.example")
	headers.Set("Authorization", "Bearer caller-secret")
	headers.Set("ChatGPT-Account-Id", "caller-account")
	response, err := transport.ExecuteWebSocket(context.Background(), proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/codex/responses", Header: headers,
	}, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d", response.StatusCode)
	}
	_, _ = io.Copy(io.Discard, response.Body)
}
