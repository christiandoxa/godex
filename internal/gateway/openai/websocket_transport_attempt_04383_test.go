package openai

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged Prodex 0.438.3 returns a typed precommit transport attempt outcome
// to the *outer routing owner* for both fresh and hard-affinity requests.
// This does not itself authorize owner rotation, quota mutation or replay.
func TestProdex04383BoundWebSocketUpstreamReadFailureIsTypedAttempt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy proxymodel.WebSocketPolicy
		body   string
	}{
		{name: "fresh", policy: proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}, body: `{"type":"response.create","input":[]}`},
		{name: "hard turn-state", policy: proxymodel.WebSocketPolicy{RequestTurnState: true}, body: `{"type":"response.create","x-codex-turn-state":"bound-1","input":[]}`},
		{name: "hard previous response", policy: proxymodel.WebSocketPolicy{RequestPreviousResponse: true}, body: `{"type":"response.create","previous_response_id":"resp-old","input":[]}`},
		{name: "hard compact session", policy: proxymodel.WebSocketPolicy{RequestSession: true}, body: `{"type":"response.create","session_id":"s-1","input":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
				if _, err := readWebSocketParityRequest(reader); err != nil {
					t.Errorf("read frame: %v", err)
					return
				}
				// An accepted upstream that closes *before* any text must
				// remain an uncommitted transport failure, not a 502 thrown
				// directly to the local WebSocket client delivery layer.
			})
			defer server.Close()
			transport := newWebSocketParityTransport(t, server)
			req := websocketParityRequest(tc.body, 843300, tc.policy)
			if tc.policy.RequestTurnState {
				req.Header.Set("x-codex-turn-state", "bound-1")
			}
			response, err := transport.ExecuteWebSocketMessage(context.Background(), req,
				proxymodel.Account{ID: "synthetic-owner", Home: "synthetic-home"})
			if err != nil {
				t.Fatalf("tagged 0.438.3 must return typed attempt not raw error: %v", err)
			}
			if response == nil {
				t.Fatal("nil precommit attempt outcome")
			}
			defer response.Body.Close()
			if response.FirstEventCommitted || response.StatusCode != http.StatusBadGateway ||
				response.PrecommitFailure == nil || !response.PrecommitFailure.Transport ||
				response.WebSocketResponseID != "" || response.WebSocketFrames != true {
				t.Fatalf("invalid typed precommit transport outcome: %+v", response)
			}
			bytes, err := io.ReadAll(response.Body)
			if err != nil || len(bytes) != 0 {
				t.Fatalf("upstream error leaked uncommitted bytes: body=%q err=%v", bytes, err)
			}
		})
	}
}

func TestProdex04383WebSocketFailedHandshakeReturnsTypedPrecommitAttempt(t *testing.T) {
	transport, err := NewTransport("http://127.0.0.1:1", nil, websocketAuth{})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	request := websocketParityRequest(`{"type":"response.create","x-codex-turn-state":"bound-1","input":[]}`, 843301,
		proxymodel.WebSocketPolicy{RequestTurnState: true})
	request.Header.Set("x-codex-turn-state", "bound-1")
	response, err := transport.ExecuteWebSocketMessage(context.Background(), request,
		proxymodel.Account{ID: "synthetic-owner", Home: "synthetic-home"})
	if err != nil {
		t.Fatalf("0.438.3 connection failure bypassed retry owner: %v", err)
	}
	if response == nil {
		t.Fatal("nil failed handshake outcome")
	}
	defer response.Body.Close()
	if response.FirstEventCommitted || response.StatusCode != http.StatusBadGateway ||
		response.PrecommitFailure == nil || !response.PrecommitFailure.Transport ||
		strings.TrimSpace(response.WebSocketTurnState) != "" {
		t.Fatalf("invalid handshake attempt classification: %+v", response)
	}
}

func TestProdex04383PartialPrecommitWebSocketFrameIsTypedNotCommitted(t *testing.T) {
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		// Finite frame header promises 12 payload bytes. Sending only the
		// first two before EOF must never leak those partial model bytes as
		// a committed response or fail the outer owner without a typed cause.
		_, _ = connection.Write([]byte{0x81, 0x0c, '{', '"'})
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	account := proxymodel.Account{ID: "synthetic-owner", Home: "synthetic-home"}
	request := websocketParityRequest(`{"type":"response.create","x-codex-turn-state":"turn-old","input":[]}`, 843302,
		proxymodel.WebSocketPolicy{RequestTurnState: true})
	request.Header.Set("x-codex-turn-state", "turn-old")
	result, err := transport.ExecuteWebSocketMessage(context.Background(), request, account)
	if err != nil {
		t.Fatalf("partial upstream frame skipped outer recovery: %v", err)
	}
	if result == nil {
		t.Fatal("missing typed failed attempt")
	}
	defer result.Body.Close()
	if result.StatusCode != http.StatusBadGateway || result.FirstEventCommitted ||
		result.PrecommitFailure == nil || !result.PrecommitFailure.Transport {
		t.Fatalf("partial upstream frame was committed/misclassified: %+v", result)
	}
	visible, err := io.ReadAll(result.Body)
	if err != nil || len(visible) != 0 {
		t.Fatalf("partial bytes escaped precommit boundary: %q err=%v", visible, err)
	}
}
