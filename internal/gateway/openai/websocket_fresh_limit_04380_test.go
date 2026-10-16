package openai

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Pinned Prodex 0.438.0 real-binary oracle: an uncommitted connection-limit
// on a FRESH WebSocket gets one same-owner new-connection attempt, not only a
// retry on a previously recycled WebSocket. The followup request is byte-
// identical and a persistent limit cannot spin indefinitely or rotate owner.
func TestProdex04380FreshWebSocketConnectionLimitRetriesOnlyBeforeCommit(t *testing.T) {
	for _, testCase := range []struct {
		name, limit    string
		persistent     bool
		alreadyRetried bool
		wantAttempts   int32
	}{
		{name: "top-level limit beats unrelated nested error", limit: `{"type":"error","error":{"code":"unrelated_failure"},"code":"WEBSOCKET_CONNECTION_LIMIT_REACHED"}`, wantAttempts: 2},
		{name: "nested limit beats unrelated top code", limit: `{"type":"error","error":{"code":"WEBSOCKET_CONNECTION_LIMIT_REACHED"},"code":"unrelated_failure"}`, wantAttempts: 2},
		{name: "persistent limit is bounded", limit: `{"type":"error","code":"websocket_connection_limit_reached"}`, persistent: true, wantAttempts: 2},
		{name: "already retried never retries again", limit: `{"type":"error","code":"websocket_connection_limit_reached"}`, alreadyRetried: true, wantAttempts: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var attempts atomic.Int32
			requests := make(chan string, 3)
			server := newWebSocketMessageTestServer(t, func(conn net.Conn, reader *bufio.Reader, _ *http.Request) {
				request, err := readWebSocketTestRequest(reader)
				if err != nil {
					t.Errorf("upstream request: %v", err)
					return
				}
				requests <- string(request)
				number := attempts.Add(1)
				reply := testCase.limit
				if number > 1 && !testCase.persistent {
					reply = `{"type":"response.completed","response":{"id":"resp-recovered"}}`
				}
				if err := writeWebSocketEvent(conn, []byte(reply)); err != nil {
					t.Errorf("send reply: %v", err)
				}
			}, nil)
			defer server.Close()
			transport, err := NewTransport(server.URL, nil, websocketAuth{})
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			account := proxymodel.Account{ID: "synthetic-owner", Home: "synthetic-home"}
			raw := `{"type":"response.create","response":{}}`
			request := websocketMessageRequest(raw)
			request.FirstEventRetryUsed = testCase.alreadyRetried
			response, err := transport.ExecuteWebSocketMessage(t.Context(), request, account)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, err := io.ReadAll(io.LimitReader(response.Body, 8192))
			if err != nil {
				t.Fatal(err)
			}
			if attempts.Load() != testCase.wantAttempts {
				t.Fatalf("upstream attempts=%d, want=%d, response=%q", attempts.Load(), testCase.wantAttempts, data)
			}
			for i := int32(0); i < testCase.wantAttempts; i++ {
				if got := <-requests; got != raw {
					t.Fatalf("replayed request body changed: %q", got)
				}
			}
			if !response.FirstEventRetryUsed {
				t.Fatal("connection limit retry budget was not propagated")
			}
			if testCase.persistent || testCase.alreadyRetried {
				if response.PrecommitFailure == nil || response.FirstEventCommitted ||
					!strings.EqualFold(response.PrecommitFailure.Code, "websocket_connection_limit_reached") {
					t.Fatalf("bounded repeated limit misclassified: %+v", response)
				}
			} else if response.PrecommitFailure != nil || !response.FirstEventCommitted || !strings.Contains(string(data), "resp-recovered") {
				t.Fatalf("fresh limit did not recover to committed response: %+v data=%s", response, data)
			}
		})
	}
}

// A limit that arrives *after visible output* cannot replay or consume an
// extra upstream turn. Existing native Codex output must be retained once.
func TestProdex04380FreshWebSocketLimitAfterCommittedDeltaNeverReplays(t *testing.T) {
	var handshakes atomic.Int32
	server := newWebSocketMessageTestServer(t, func(conn net.Conn, reader *bufio.Reader, _ *http.Request) {
		handshakes.Add(1)
		if _, err := readWebSocketTestRequest(reader); err != nil {
			t.Errorf("read: %v", err)
			return
		}
		for _, frame := range []string{
			`{"type":"response.output_text.delta","delta":"visible-once"}`,
			`{"type":"error","code":"websocket_connection_limit_reached"}`,
		} {
			if err := writeWebSocketEvent(conn, []byte(frame)); err != nil {
				return
			}
		}
	}, nil)
	defer server.Close()
	transport, err := NewTransport(server.URL, nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	req := websocketMessageRequest(`{"type":"response.create","response":{}}`)
	response, err := transport.ExecuteWebSocketMessage(t.Context(), req, proxymodel.Account{ID: "profile-a", Home: "synthetic-home"})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !response.FirstEventCommitted || response.PrecommitFailure != nil || response.FirstEventRetryUsed {
		t.Fatalf("already-committed output became retryable: %+v", response)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 8192))
	if err != nil {
		t.Fatal(err)
	}
	if handshakes.Load() != 1 || !strings.Contains(string(content), "visible-once") ||
		strings.Count(string(content), "visible-once") != 1 {
		t.Fatalf("committed output replayed or lost: attempts=%d frames=%q", handshakes.Load(), content)
	}
}
