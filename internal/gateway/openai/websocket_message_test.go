package openai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
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
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestExecuteWebSocketMessageStreamsThroughTerminalEvent(t *testing.T) {
	requestText := `{"type":"response.create","response":{"model":"synthetic-model"}}`
	terminal := []byte(`{"response":{"id":"resp_test","padding":"` + strings.Repeat("x", websocketEventTypePrefixBytes*2) + `"},"type":"response.completed"}`)
	want := [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_test"}}`),
		[]byte(`{"type":"response.output_text.delta","delta":"ready"}`),
		terminal,
	}
	requestSeen := make(chan string, 1)
	server := newWebSocketMessageTestServer(t, func(connection net.Conn, reader *bufio.Reader, request *http.Request) {
		frame, err := websocketframe.ReadHeader(reader)
		if err != nil {
			t.Errorf("read request frame: %v", err)
			return
		}
		payload, err := frame.ReadPayload(reader, 1<<20)
		if err != nil {
			t.Errorf("read request payload: %v", err)
			return
		}
		frame.Unmask(payload)
		requestSeen <- string(payload)
		for _, event := range want {
			if err := writeWebSocketEvent(connection, event); err != nil {
				t.Errorf("write response event: %v", err)
				return
			}
		}
		// The client is allowed to close the upstream socket immediately after the
		// terminal event. If this write succeeds, the assertion below still proves
		// the post-terminal event was not forwarded; if it fails, closure is the
		// expected equivalent outcome.
		_ = writeWebSocketEvent(connection, []byte(`{"type":"response.created","response":{"id":"must_not_forward"}}`))
	})
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	response, err := transport.ExecuteWebSocketMessage(context.Background(), websocketMessageRequest(requestText), proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !response.WebSocketFrames || !response.FirstEventCommitted || response.WebSocketResponseID != "resp_test" {
		t.Fatalf("message response = %#v", response)
	}
	if got := <-requestSeen; got != requestText {
		t.Fatalf("upstream request message = %q", got)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	wantFrames := websocketEventFrames(t, want...)
	if !bytes.Equal(body, wantFrames) {
		t.Fatalf("response frames differ: got %d bytes, want %d", len(body), len(wantFrames))
	}
}

func TestWebSocketEventTypePrefixSkipsLargeNestedFields(t *testing.T) {
	payload := []byte(`{"response":{"padding":"` + strings.Repeat("x", websocketEventTypePrefixBytes*2) + `"},"type":"response.completed"}`)
	if got := websocketEventTypePrefix(payload); got != "response.completed" {
		t.Fatalf("event type = %q", got)
	}
}

func TestWebSocketFailureCodePrefersSpecificCodeOverGenericType(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"previous_response_not_found"}}}`)
	if got := websocketFailureCode(payload); got != "previous_response_not_found" {
		t.Fatalf("failure code = %q", got)
	}
}

func TestExecuteWebSocketMessageReusesUpstreamSession(t *testing.T) {
	var handshakes atomic.Int32
	requests := make(chan string, 2)
	server := newWebSocketMessageTestServer(t, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		handshakes.Add(1)
		for index := 1; index <= 2; index++ {
			payload, err := readWebSocketTestRequest(reader)
			if err != nil {
				t.Errorf("read request %d: %v", index, err)
				return
			}
			requests <- string(payload)
			event := []byte(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_%d"}}`, index))
			if err := writeWebSocketEvent(connection, event); err != nil {
				t.Errorf("write response %d: %v", index, err)
				return
			}
		}
		_, _ = reader.ReadByte()
	})
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}
	for index, body := range []string{`{"type":"response.create","response":{}}`, `{"type":"response.create","response":{"previous_response_id":"resp_1"}}`} {
		request := websocketMessageRequest(body)
		request.WebSocketSessionID = 41
		response, err := transport.ExecuteWebSocketMessage(context.Background(), request, account)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			t.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if got := <-requests; got != body {
			t.Fatalf("upstream request %d = %q, want %q", index+1, got, body)
		}
	}
	if got := handshakes.Load(); got != 1 {
		t.Fatalf("upstream handshakes = %d, want 1", got)
	}
	transport.CloseWebSocketSession(41)
}

func TestExecuteWebSocketMessageCarriesTurnStateFromReusedSession(t *testing.T) {
	requests := make(chan string, 2)
	server := newWebSocketMessageTestServer(t, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		for index, response := range []string{
			`{"type":"response.completed","response":{"id":"resp_first"}}`,
			`{"type":"response.failed","response":{"error":{"code":"previous_response_not_found"}}}`,
		} {
			payload, err := readWebSocketTestRequest(reader)
			if err != nil {
				t.Errorf("read request %d: %v", index+1, err)
				return
			}
			requests <- string(payload)
			if err := writeWebSocketEvent(connection, []byte(response)); err != nil {
				t.Errorf("write response %d: %v", index+1, err)
				return
			}
		}
	}, http.Header{"X-Codex-Turn-State": {"turn-state-owner"}})
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}

	for index, body := range []string{
		`{"type":"response.create","response":{}}`,
		`{"type":"response.create","response":{"previous_response_id":"resp_first"}}`,
	} {
		request := websocketMessageRequest(body)
		request.WebSocketSessionID = 42
		response, err := transport.ExecuteWebSocketMessage(context.Background(), request, account)
		if err != nil {
			t.Fatal(err)
		}
		if response.WebSocketTurnState != "turn-state-owner" {
			t.Fatalf("request %d did not preserve the upstream turn state", index+1)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if got := <-requests; got != body {
			t.Fatalf("request %d = %q, want %q", index+1, got, body)
		}
		if index == 1 && (response.PrecommitFailure == nil || response.PrecommitFailure.Code != "previous_response_not_found") {
			t.Fatalf("reused-session failure = %#v", response.PrecommitFailure)
		}
	}
}

func TestExecuteWebSocketMessageRetainsLargeTurnState(t *testing.T) {
	turnState := strings.Repeat("x", 8<<10)
	server := newWebSocketMessageTestServer(t, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if err := consumeWebSocketTestRequest(reader); err != nil {
			t.Errorf("read request frame: %v", err)
			return
		}
		if err := writeWebSocketEvent(connection, []byte(`{"type":"response.completed"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}, http.Header{"X-Codex-Turn-State": {turnState}})
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	request := websocketMessageRequest(`{"type":"response.create"}`)
	request.WebSocketSessionID = 43
	response, err := transport.ExecuteWebSocketMessage(context.Background(), request, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	transport.websocketMessageMu.Lock()
	retained := transport.websocketMessageSessions[43].turnState
	transport.websocketMessageMu.Unlock()
	if response.WebSocketTurnState != turnState || retained != turnState {
		t.Fatal("large turn state was not retained")
	}
}

func TestTakeWebSocketMessageSessionMatchesAccountAndTurnStateOverride(t *testing.T) {
	transport, err := NewTransport("http://127.0.0.1", nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	client, peer := net.Pipe()
	defer peer.Close()
	transport.websocketMessageMu.Lock()
	transport.websocketMessageSessions[9] = websocketMessageSession{
		connection: client, accountID: "profile-a", home: "synthetic-home", turnState: "turn-a",
		completed: time.Now().Add(-2 * time.Minute),
	}
	transport.websocketMessageMu.Unlock()
	stale := transport.takeWebSocketMessageSession(9, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}, "")
	if stale.connection == nil {
		t.Fatal("gateway rejected idle session before routing could apply stale continuation policy")
	}
	_ = stale.connection.Close()

	for _, test := range []struct {
		name, accountID, home, override string
	}{
		{name: "wrong account", accountID: "profile-b", home: "synthetic-home"},
		{name: "wrong home", accountID: "profile-a", home: "other-home"},
		{name: "turn state mismatch", accountID: "profile-a", home: "synthetic-home", override: "turn-b"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, peer := net.Pipe()
			defer peer.Close()
			transport.websocketMessageMu.Lock()
			transport.websocketMessageSessions[10] = websocketMessageSession{
				connection: client, accountID: "profile-a", home: "synthetic-home", turnState: "turn-a", completed: time.Now(),
			}
			transport.websocketMessageMu.Unlock()
			got := transport.takeWebSocketMessageSession(10, proxymodel.Account{ID: test.accountID, Home: test.home}, test.override)
			if got.connection != nil {
				_ = got.connection.Close()
				t.Fatal("mismatched websocket session was reused")
			}
		})
	}
}

func TestCloseWebSocketSessionOwnsSessionLifecycle(t *testing.T) {
	transport, err := NewTransport("http://127.0.0.1", nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}
	client, peer := net.Pipe()
	defer peer.Close()
	transport.recycleWebSocketMessageSession(77, account, client, "turn-a")
	transport.CloseWebSocketSession(77)
	transport.websocketMessageMu.Lock()
	_, exists := transport.websocketMessageSessions[77]
	transport.websocketMessageMu.Unlock()
	if exists {
		t.Fatal("closed websocket session remained retained")
	}
}

func TestExecuteWebSocketMessageReconnectsOnReusedConnectionLimit(t *testing.T) {
	var handshakes atomic.Int32
	requests := make(chan string, 3)
	server := newWebSocketMessageTestServer(t, func(connection net.Conn, reader *bufio.Reader, request *http.Request) {
		switch handshakes.Add(1) {
		case 1:
			for _, response := range []string{
				`{"type":"response.completed","response":{"id":"resp_reuse"}}`,
				`{"type":"error","error":{"code":"websocket_connection_limit_reached"}}`,
			} {
				payload, err := readWebSocketTestRequest(reader)
				if err != nil {
					t.Errorf("read reused-session request: %v", err)
					return
				}
				requests <- string(payload)
				if err := writeWebSocketEvent(connection, []byte(response)); err != nil {
					t.Errorf("write reused-session event: %v", err)
					return
				}
			}
		case 2:
			if got := request.Header.Get("x-codex-turn-state"); got != "turn-state-owner" {
				t.Error("fresh retry did not carry the upstream turn state")
			}
			payload, err := readWebSocketTestRequest(reader)
			if err != nil {
				t.Errorf("read fresh-session retry: %v", err)
				return
			}
			requests <- string(payload)
			if err := writeWebSocketEvent(connection, []byte(`{"type":"response.completed","response":{"id":"resp_retry"}}`)); err != nil {
				t.Errorf("write fresh-session response: %v", err)
			}
		}
	}, http.Header{"X-Codex-Turn-State": {"turn-state-owner"}})
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}
	for _, body := range []string{
		`{"type":"response.create","response":{}}`,
		`{"type":"response.create","response":{"previous_response_id":"resp_reuse"}}`,
	} {
		request := websocketMessageRequest(body)
		request.WebSocketSessionID = 42
		response, err := transport.ExecuteWebSocketMessage(context.Background(), request, account)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			t.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if got := <-requests; got != body {
			t.Fatalf("upstream request = %q, want %q", got, body)
		}
		if strings.Contains(body, "previous_response_id") && !response.FirstEventRetryUsed {
			t.Fatal("connection-limit retry was not recorded")
		}
	}
	if got := <-requests; got != `{"type":"response.create","response":{"previous_response_id":"resp_reuse"}}` {
		t.Fatalf("fresh-session retry = %q", got)
	}
	if got := handshakes.Load(); got != 2 {
		t.Fatalf("upstream handshakes = %d, want 2", got)
	}
}

func TestExecuteWebSocketMessageReturnsPrecommitFailure(t *testing.T) {
	want := [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp_failed"}}`),
		[]byte(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded"}}`),
	}
	server := newWebSocketMessageTestServer(t, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if err := consumeWebSocketTestRequest(reader); err != nil {
			t.Errorf("read request frame: %v", err)
			return
		}
		for _, event := range want {
			if err := writeWebSocketEvent(connection, event); err != nil {
				t.Errorf("write response event: %v", err)
				return
			}
		}
	})
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.ExecuteWebSocketMessage(context.Background(), websocketMessageRequest(`{"type":"response.create"}`), proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.FirstEventCommitted || response.PrecommitFailure == nil || response.PrecommitFailure.Code != "rate_limit_exceeded" || response.WebSocketResponseID != "resp_failed" {
		t.Fatalf("precommit response = %#v", response)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	wantFrames := websocketEventFrames(t, want[len(want)-1])
	if !bytes.Equal(body, wantFrames) {
		t.Fatalf("precommit failure exposed held frames: got %d bytes, want %d", len(body), len(wantFrames))
	}
}

func TestExecuteWebSocketMessageStreamsAfterPrecommitLimit(t *testing.T) {
	large := []byte(`{"type":"response.created","padding":"` + strings.Repeat("x", websocketPrecommitLookaheadBytes) + `"}`)
	want := [][]byte{
		large,
		[]byte(`{"type":"response.completed","response":{"id":"resp_large"}}`),
	}
	server := newWebSocketMessageTestServer(t, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if err := consumeWebSocketTestRequest(reader); err != nil {
			t.Errorf("read request frame: %v", err)
			return
		}
		for _, event := range want {
			if err := writeWebSocketEvent(connection, event); err != nil {
				t.Errorf("write response event: %v", err)
				return
			}
		}
	})
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.ExecuteWebSocketMessage(context.Background(), websocketMessageRequest(`{"type":"response.create"}`), proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !response.FirstEventCommitted || !response.WebSocketFrames {
		t.Fatalf("large precommit response = %#v", response)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if wantFrames := websocketEventFrames(t, want...); !bytes.Equal(body, wantFrames) {
		t.Fatalf("streamed response frames differ: got %d bytes, want %d", len(body), len(wantFrames))
	}
}

func websocketMessageRequest(body string) proxymodel.Request {
	return proxymodel.Request{
		Method: http.MethodGet, Path: "/backend-api/codex/responses",
		Header: http.Header{
			"Connection": {"Upgrade"}, "Upgrade": {"websocket"},
			"Sec-WebSocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="}, "Sec-WebSocket-Version": {"13"},
		},
		Body: []byte(body), WebSocketMessage: true,
		WebSocketPolicy: proxymodel.WebSocketPolicy{PromoteCommittedProfile: true},
	}
}

func newWebSocketMessageTestServer(
	t *testing.T,
	handle func(net.Conn, *bufio.Reader, *http.Request),
	responseHeaders ...http.Header,
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
		_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n", base64.StdEncoding.EncodeToString(digest[:]))
		for _, headers := range responseHeaders {
			for name, values := range headers {
				for _, value := range values {
					_, _ = fmt.Fprintf(buffered, "%s: %s\r\n", name, value)
				}
			}
		}
		_, _ = io.WriteString(buffered, "\r\n")
		if err := buffered.Flush(); err != nil {
			t.Errorf("flush websocket handshake: %v", err)
			return
		}
		handle(connection, buffered.Reader, request)
	}))
}

func writeWebSocketEvent(writer io.Writer, payload []byte) error {
	return websocketframe.WriteFrame(writer, 1, payload, false)
}

func consumeWebSocketTestRequest(reader *bufio.Reader) error {
	_, err := readWebSocketTestRequest(reader)
	return err
}

func readWebSocketTestRequest(reader *bufio.Reader) ([]byte, error) {
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

func websocketEventFrames(t *testing.T, payloads ...[]byte) []byte {
	t.Helper()
	var frames bytes.Buffer
	for _, payload := range payloads {
		if err := writeWebSocketEvent(&frames, payload); err != nil {
			t.Fatal(err)
		}
	}
	return frames.Bytes()
}
