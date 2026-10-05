package proxy

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const websocketTestKey = "dGhlIHNhbXBsZSBub25jZQ=="

func TestSupportedWebSocketPaths(t *testing.T) {
	for _, path := range []string{
		"/v1/realtime",
		"/v1/live",
		"/backend-api/codex/responses",
		"/backend-api/prodex/v0.2.99/responses",
		"/backend-api/codex/realtime",
		"/backend-api/prodex/v0.2.99/realtime",
		"/backend-api/codex/live/call-123",
		"/backend-api/prodex/v0.2.99/live/call-123",
	} {
		if !supportedWebSocketPath(path) {
			t.Errorf("supported websocket path rejected: %s", path)
		}
	}
	for _, path := range []string{
		"/responses",
		"/v1/responses",
		"/backend-api/prodex/v0.2.99/realtime/calls",
		"/backend-api/codex/live/call-123/extra",
		"/unrelated",
	} {
		if supportedWebSocketPath(path) {
			t.Errorf("unsupported websocket path accepted: %s", path)
		}
	}
}

func TestProxyWebSocketForwardsHandshakeFragmentedAndControlFrames(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "managed-secret", "B", "token-b")
	fragmentedText := append(maskedWebSocketFrame(1, false, "hel"), maskedWebSocketFrame(0, true, "lo")...)
	clientFrames := append(maskedWebSocketFrame(9, true, "ping"), fragmentedText...)
	seen := make(chan *http.Request, 1)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamCalls.Add(1)
		seen <- request.Clone(context.Background())
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\nSec-WebSocket-Protocol: codex.realtime\r\nSec-WebSocket-Extensions: permessage-deflate\r\nAuthorization: Bearer managed-secret\r\nSet-Cookie: __Host-session=managed-cookie; Secure; HttpOnly; Path=/\r\nX-Upstream-Handshake: ready\r\n\r\n", websocketAccept(request.Header.Get("Sec-WebSocket-Key")))
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush upstream handshake: %v", err)
			return
		}
		opcode, payload, err := readWebSocketTestFrame(buffered.Reader)
		if err != nil || opcode != 1 || string(payload) != "hello" {
			t.Errorf("semantic upstream message = opcode %d payload %q err=%v", opcode, payload, err)
			return
		}
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)

	connection, reader := dialWebSocket(t, proxy.URL, "/backend-api/codex/realtime?transport=websocket", "Bearer caller-secret")
	defer connection.Close()
	status, headers, raw := readWebSocketHandshake(t, reader)
	if status != http.StatusSwitchingProtocols || headers.Get("Upgrade") != "websocket" ||
		!strings.EqualFold(headers.Get("Connection"), "Upgrade") ||
		headers.Get("Sec-WebSocket-Accept") != websocketAccept(websocketTestKey) ||
		headers.Get("Sec-WebSocket-Protocol") != "" || headers.Get("Sec-WebSocket-Extensions") != "" ||
		headers.Get("X-Upstream-Handshake") != "" || headers.Get("Authorization") != "" ||
		len(headers.Values("Set-Cookie")) != 0 {
		t.Fatalf("downstream handshake = status %d, headers %#v", status, headers)
	}
	if strings.Contains(raw, "managed-secret") || strings.Contains(raw, "caller-secret") || strings.Contains(raw, "managed-cookie") {
		t.Fatalf("websocket handshake leaked credentials: %q", raw)
	}

	select {
	case request := <-seen:
		t.Fatalf("upstream selected before first complete text message: %s", request.URL)
	default:
	}
	if _, err := connection.Write(clientFrames); err != nil {
		t.Fatal(err)
	}
	var upstreamRequest *http.Request
	select {
	case upstreamRequest = <-seen:
	case <-time.After(time.Second):
		t.Fatal("fragmented text did not select upstream")
	}
	upstreamKey := upstreamRequest.Header.Get("Sec-WebSocket-Key")
	decodedUpstreamKey, decodeErr := base64.StdEncoding.DecodeString(upstreamKey)
	if upstreamRequest.URL.RequestURI() != "/backend-api/codex/realtime?transport=websocket" ||
		upstreamRequest.Header.Get("Authorization") != "Bearer managed-secret" ||
		upstreamRequest.Header.Get("ChatGPT-Account-Id") != "workspace-A" ||
		upstreamRequest.Header.Get("Connection") != "Upgrade" || upstreamRequest.Header.Get("Upgrade") != "websocket" ||
		decodeErr != nil || len(decodedUpstreamKey) != 16 || upstreamKey == websocketTestKey ||
		upstreamRequest.Header.Get("Sec-WebSocket-Protocol") != "" || upstreamRequest.Header.Get("Sec-WebSocket-Extensions") != "" ||
		upstreamRequest.Header.Get("Origin") != "https://client.example" {
		t.Fatalf("upstream websocket request = %s %#v", upstreamRequest.URL, upstreamRequest.Header)
	}
	if upstreamRequest.Header.Get("Authorization") == "Bearer caller-secret" {
		t.Fatal("caller credential reached upstream")
	}
	opcode, payload, err := readWebSocketTestFrame(reader)
	if err != nil || opcode != 10 || string(payload) != "ping" {
		t.Fatalf("local ping handling = opcode %d payload %q err=%v", opcode, payload, err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("upstream close did not close downstream websocket")
	} else {
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			t.Fatalf("downstream websocket stayed open after upstream close: %v", err)
		}
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("websocket rotated after upgrade commitment: upstream calls = %d", upstreamCalls.Load())
	}
}

func TestProxyWebSocketRejectsBinaryMessagesAndContinues(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		frames []byte
	}{
		{name: "single frame", frames: maskedWebSocketFrame(2, true, "binary")},
		{name: "fragmented message", frames: append(maskedWebSocketFrame(2, false, "bin"), maskedWebSocketFrame(0, true, "ary")...)},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			accounts := testRuntimeAccounts(t, "A", "managed-secret", "B", "token-b")
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				connection, buffered, err := writer.(http.Hijacker).Hijack()
				if err != nil {
					t.Errorf("hijack upstream: %v", err)
					return
				}
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(time.Second))
				_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", websocketAccept(request.Header.Get("Sec-WebSocket-Key")))
				if err := buffered.Flush(); err != nil {
					t.Errorf("flush upstream handshake: %v", err)
					return
				}
				opcode, payload, err := readWebSocketTestFrame(buffered.Reader)
				if err != nil || opcode != 1 || string(payload) != "hello" {
					t.Errorf("upstream frame = opcode %d, payload %q, error %v", opcode, payload, err)
					return
				}
				response := `{"type":"response.completed","response":{"id":"resp_hello"}}`
				_, _ = buffered.Write(unmaskedWebSocketFrame(1, true, response))
				_ = buffered.Flush()
			}))
			defer upstream.Close()
			proxy := newTestProxy(t, upstream.URL, accounts)

			connection, reader := dialWebSocket(t, proxy.URL, "/backend-api/codex/responses", "")
			defer connection.Close()
			status, _, _ := readWebSocketHandshake(t, reader)
			if status != http.StatusSwitchingProtocols {
				t.Fatalf("websocket status = %d", status)
			}
			frames := append(fixture.frames, maskedWebSocketFrame(1, true, "hello")...)
			if _, err := connection.Write(frames); err != nil {
				t.Fatal(err)
			}
			_ = connection.SetReadDeadline(time.Now().Add(time.Second))
			opcode, payload, err := readWebSocketTestFrame(reader)
			if err != nil || opcode != 1 || !strings.Contains(string(payload), `"type":"error"`) ||
				!strings.Contains(string(payload), `"status":400`) || !strings.Contains(string(payload), `"code":"invalid_request_error"`) {
				t.Fatalf("binary rejection = opcode %d, payload %q, error %v", opcode, payload, err)
			}
			opcode, payload, err = readWebSocketTestFrame(reader)
			if err != nil || opcode != 1 || string(payload) != `{"type":"response.completed","response":{"id":"resp_hello"}}` {
				t.Fatalf("continued response = opcode %d, payload %q, error %v", opcode, payload, err)
			}
		})
	}
}

func TestProxyWebSocketUpstreamHandshakeRejectionBecomesPostUpgradeMessage(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "managed-secret", "B", "token-b")
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Upstream-Handshake", "rejected")
		writer.Header().Add("Set-Cookie", "session=upstream-secret; Path=/; HttpOnly")
		writer.Header().Set("Content-Type", "text/plain")
		writer.Header().Set("Content-Length", "13")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(writer, "invalid model")
	}))
	defer upstream.Close()
	proxy := newTestProxy(t, upstream.URL, accounts)
	connection, reader := dialWebSocket(t, proxy.URL, "/v1/realtime?model=synthetic", "")
	defer connection.Close()
	status, headers, _ := readWebSocketHandshake(t, reader)
	if status != http.StatusSwitchingProtocols || headers.Get("X-Upstream-Handshake") != "" || len(headers.Values("Set-Cookie")) != 0 {
		t.Fatalf("local handshake = status %d headers %#v", status, headers)
	}
	if _, err := connection.Write(maskedWebSocketFrame(1, true, `{"type":"session.update"}`)); err != nil {
		t.Fatal(err)
	}
	opcode, payload, err := readWebSocketTestFrame(reader)
	if err != nil || opcode != 1 || string(payload) != "invalid model" {
		t.Fatalf("post-upgrade rejection = opcode %d payload %q err=%v", opcode, payload, err)
	}
}

func TestProxyCloseCancelsActiveWebSocketTunnel(t *testing.T) {
	accounts := testRuntimeAccounts(t, "A", "managed-secret", "B", "token-b")
	upstreamReady := make(chan struct{})
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, buffered, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack upstream: %v", err)
			return
		}
		defer connection.Close()
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", websocketAccept(request.Header.Get("Sec-WebSocket-Key")))
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush upstream handshake: %v", err)
			return
		}
		opcode, payload, err := readWebSocketTestFrame(buffered.Reader)
		if err != nil || opcode != 1 || string(payload) != `{"type":"session.update"}` {
			t.Errorf("upstream first realtime message = opcode %d payload %q err=%v", opcode, payload, err)
			return
		}
		close(upstreamReady)
		_, _ = buffered.Reader.ReadByte()
		close(upstreamClosed)
	}))
	defer upstream.Close()
	proxy, err := newProxyForTest(ProxyConfig{UpstreamURL: upstream.URL, Accounts: func(context.Context) ([]RuntimeAccount, error) { return accounts, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	connection, reader := dialWebSocket(t, proxy.Endpoint(), "/backend-api/codex/realtime", "")
	defer connection.Close()
	status, _, _ := readWebSocketHandshake(t, reader)
	if status != http.StatusSwitchingProtocols {
		t.Fatalf("handshake status = %d", status)
	}
	if _, err := connection.Write(maskedWebSocketFrame(1, true, `{"type":"session.update"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-upstreamReady:
	case <-time.After(time.Second):
		t.Fatal("first realtime text did not open upstream websocket")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := proxy.Close(ctx); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("proxy close left downstream websocket open")
	}
	select {
	case <-upstreamClosed:
	case <-time.After(time.Second):
		t.Fatal("proxy close left upstream websocket open")
	}
}

func dialWebSocket(t *testing.T, endpoint, path, authorization string) (net.Conn, *bufio.Reader) {
	t.Helper()
	parsed, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.Dial("tcp", parsed.URL.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	_, err = fmt.Fprintf(connection, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: keep-alive, Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Protocol: codex.realtime\r\nSec-WebSocket-Extensions: permessage-deflate\r\nOrigin: https://client.example\r\nAuthorization: %s\r\nChatGPT-Account-Id: caller-account\r\n\r\n", path, parsed.URL.Host, websocketTestKey, authorization)
	if err != nil {
		t.Fatal(err)
	}
	return connection, bufio.NewReader(connection)
}

func readWebSocketHandshake(t *testing.T, reader *bufio.Reader) (int, http.Header, string) {
	t.Helper()
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var status int
	if _, err := fmt.Sscanf(statusLine, "HTTP/1.1 %d", &status); err != nil {
		t.Fatalf("invalid websocket status line %q: %v", statusLine, err)
	}
	var headers http.Header = make(http.Header)
	var raw strings.Builder
	raw.WriteString(statusLine)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		raw.WriteString(line)
		if line == "\r\n" {
			return status, headers, raw.String()
		}
		name, value, ok := strings.Cut(strings.TrimRight(line, "\r\n"), ":")
		if !ok {
			t.Fatalf("invalid websocket header %q", line)
		}
		headers.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}
}

func readWebSocketTestFrame(reader *bufio.Reader) (byte, []byte, error) {
	frame, err := websocketframe.ReadHeader(reader)
	if err != nil {
		return 0, nil, err
	}
	payload, err := frame.ReadPayload(reader, 1<<20)
	if err != nil {
		return 0, nil, err
	}
	frame.Unmask(payload)
	return frame.Opcode, payload, nil
}

func websocketAccept(key string) string {
	digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(digest[:])
}

func maskedWebSocketFrame(opcode byte, final bool, payload string) []byte {
	frame := unmaskedWebSocketFrame(opcode, final, payload)
	frame[1] |= 0x80
	mask := [4]byte{1, 2, 3, 4}
	frame = append(frame[:2], append(mask[:], frame[2:]...)...)
	for index := 0; index < len(payload); index++ {
		frame[6+index] ^= mask[index%len(mask)]
	}
	return frame
}

func unmaskedWebSocketFrame(opcode byte, final bool, payload string) []byte {
	first := opcode
	if final {
		first |= 0x80
	}
	return append([]byte{first, byte(len(payload))}, []byte(payload)...)
}
