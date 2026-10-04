package openai

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestTerminalEventTurnStateIsRetainedForSessionReuse(t *testing.T) {
	var handshakes atomic.Int32
	server := newWebSocketMessageParityServer(
		t,
		http.Header{"X-Codex-Turn-State": {"handshake-state"}},
		func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
			handshakes.Add(1)
			for index := 0; index < 2; index++ {
				if _, err := readWebSocketParityRequest(reader); err != nil {
					t.Errorf("read request %d: %v", index, err)
					return
				}
				turnState := "event-state"
				if index == 1 {
					turnState = "event-state-2"
				}
				payload := []byte(
					"{\"type\":\"response.completed\",\"response\":{\"id\":\"resp_done\"},\"turn_state\":\"" +
						turnState + "\"}",
				)
				if err := writeWebSocketParityEvent(connection, payload); err != nil {
					t.Errorf("write response %d: %v", index, err)
					return
				}
			}
		},
	)
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}

	first, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest(
			"{\"type\":\"response.create\"}",
			90,
			proxymodel.WebSocketPolicy{PromoteCommittedProfile: true},
		),
		account,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first.WebSocketTurnState != "event-state" {
		t.Fatalf("first turn state = %q", first.WebSocketTurnState)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	if err := first.Body.Close(); err != nil {
		t.Fatal(err)
	}

	secondRequest := websocketParityRequest(
		"{\"type\":\"response.create\"}",
		90,
		proxymodel.WebSocketPolicy{
			PromoteCommittedProfile: true,
			TurnStateOverride:       true,
		},
	)
	secondRequest.Header.Set("x-codex-turn-state", "event-state")
	second, err := transport.ExecuteWebSocketMessage(context.Background(), secondRequest, account)
	if err != nil {
		t.Fatal(err)
	}
	if !second.WebSocketReusedSession || second.WebSocketTurnState != "event-state-2" {
		t.Fatalf(
			"second response reused=%t turn-state=%q",
			second.WebSocketReusedSession,
			second.WebSocketTurnState,
		)
	}
	_, _ = io.Copy(io.Discard, second.Body)
	_ = second.Body.Close()
	if handshakes.Load() != 1 {
		t.Fatalf("upstream handshakes = %d, want 1", handshakes.Load())
	}
}

func TestTurnStateOverrideMismatchForcesFreshUpstreamSession(t *testing.T) {
	var handshakes atomic.Int32
	server := newWebSocketMessageParityServer(
		t,
		http.Header{"X-Codex-Turn-State": {"turn-a"}},
		func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
			handshake := handshakes.Add(1)
			if _, err := readWebSocketParityRequest(reader); err != nil {
				t.Errorf("read request: %v", err)
				return
			}
			payload := []byte("{\"type\":\"response.completed\",\"response\":{\"id\":\"resp_done\"}}")
			if handshake == 2 {
				payload = []byte(
					"{\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fresh\"},\"turn_state\":\"turn-b\"}",
				)
			}
			if err := writeWebSocketParityEvent(connection, payload); err != nil {
				t.Errorf("write response: %v", err)
			}
		},
	)
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	account := proxymodel.Account{ID: "profile-a", Home: "synthetic-home"}

	first, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest(
			"{\"type\":\"response.create\"}",
			91,
			proxymodel.WebSocketPolicy{PromoteCommittedProfile: true},
		),
		account,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, first.Body)
	_ = first.Body.Close()

	secondRequest := websocketParityRequest(
		"{\"type\":\"response.create\"}",
		91,
		proxymodel.WebSocketPolicy{TurnStateOverride: true},
	)
	secondRequest.Header.Set("x-codex-turn-state", "turn-b")
	second, err := transport.ExecuteWebSocketMessage(context.Background(), secondRequest, account)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()
	if second.WebSocketReusedSession {
		t.Fatal("mismatched turn-state override reused the old upstream session")
	}
	if second.WebSocketTurnState != "turn-b" {
		t.Fatalf("fresh response turn-state = %q", second.WebSocketTurnState)
	}
	if handshakes.Load() != 2 {
		t.Fatalf("upstream handshakes = %d, want 2", handshakes.Load())
	}
}
