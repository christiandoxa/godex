package openai

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestWebSocketEventInspectionMatchesProdex04353RetryAndTerminalPolicy(t *testing.T) {
	for _, test := range []struct {
		name      string
		payload   string
		retryCode string
		terminal  bool
		turnState string
	}{
		{
			name:      "quota from response headers",
			payload:   "{\"type\":\"response.failed\",\"error\":{\"code\":\"usage_not_included\",\"message\":\"quota exceeded\"},\"response\":{\"id\":\"resp_1\",\"headers\":{\"x-codex-turn-state\":\"turn-1\"}}}",
			retryCode: "usage_not_included", terminal: true, turnState: "turn-1",
		},
		{
			name:      "workspace credits quota",
			payload:   "{\"type\":\"response.failed\",\"error\":{\"code\":\"workspace_member_credits_depleted\",\"message\":\"quota exceeded\"}}",
			retryCode: "workspace_member_credits_depleted", terminal: true,
		},
		{
			name:      "profile unavailable",
			payload:   "{\"type\":\"response.failed\",\"error\":{\"code\":\"deactivated_workspace\",\"message\":\"profile unavailable\"}}",
			retryCode: "deactivated_workspace", terminal: true,
		},
		{
			name:     "wrapped invalid request commits",
			payload:  "{\"type\":\"error\",\"status_code\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Model does not support image inputs\"}}",
			terminal: true,
		},
		{
			name:     "invalid prompt response failed commits",
			payload:  "{\"type\":\"response.failed\",\"response\":{\"id\":\"resp-failed\",\"error\":{\"code\":\"invalid_prompt\",\"message\":\"Invalid request.\"}}}",
			terminal: true,
		},
		{
			name:    "non error text mentioning retry code",
			payload: "{\"type\":\"response.output_text.delta\",\"delta\":\"rate_limit_exceeded is an example\"}",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := []byte(test.payload)
			kind, _, turnState := websocketEventMetadata(raw)
			if got := websocketRetryFailureCode(raw); got != test.retryCode {
				t.Fatalf("retry code = %q, want %q", got, test.retryCode)
			}
			if got := websocketPayloadTerminal(kind, raw); got != test.terminal {
				t.Fatalf("terminal = %t, want %t", got, test.terminal)
			}
			if turnState != test.turnState {
				t.Fatalf("turn-state = %q, want %q", turnState, test.turnState)
			}
		})
	}
}

func TestNonRetryableResponseFailedCommitsAndForwardsTerminalFrame(t *testing.T) {
	failure := []byte("{\"type\":\"response.failed\",\"response\":{\"id\":\"resp-failed\",\"error\":{\"code\":\"invalid_prompt\",\"message\":\"Invalid request.\"}}}")
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = writeWebSocketParityEvent(connection, failure)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	response, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest("{\"type\":\"response.create\"}", 70, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !response.FirstEventCommitted || response.PrecommitFailure != nil {
		t.Fatalf("non-retryable response.failed was not committed: %#v", response)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := websocketParityFrames(t, failure); !bytes.Equal(body, want) {
		t.Fatalf("forwarded failure frame = %d bytes, want %d", len(body), len(want))
	}
	transport.websocketMessageMu.Lock()
	_, retained := transport.websocketMessageSessions[70]
	transport.websocketMessageMu.Unlock()
	if retained {
		t.Fatal("response.failed upstream session was recycled")
	}
}

func TestWrappedStatusErrorCommitsAndForwardsTerminalFrame(t *testing.T) {
	failure := []byte("{\"type\":\"error\",\"status_code\":400,\"error\":{\"type\":\"invalid_request_error\",\"message\":\"Model does not support image inputs\"}}")
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = writeWebSocketParityEvent(connection, failure)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	response, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest("{\"type\":\"response.create\"}", 71, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !response.FirstEventCommitted || response.PrecommitFailure != nil {
		t.Fatalf("wrapped 400 error was not committed: %#v", response)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := websocketParityFrames(t, failure); !bytes.Equal(body, want) {
		t.Fatalf("wrapped error frame = %d bytes, want %d", len(body), len(want))
	}
}

func TestUntypedTextFrameUsesCommittedIdleTimeout(t *testing.T) {
	payload := []byte("{\"message\":\"progress\"}")
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		_ = writeWebSocketParityEvent(connection, payload)
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	response, err := transport.ExecuteWebSocketMessage(
		context.Background(),
		websocketParityRequest("{\"type\":\"response.create\"}", 72, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if !response.FirstEventCommitted {
		t.Fatal("unknown text response did not commit")
	}
	body, ok := response.Body.(*websocketResponseBody)
	if !ok {
		t.Fatalf("response body type = %T", response.Body)
	}
	watchdog, ok := body.connection.(*websocketReadWatchdog)
	if !ok {
		t.Fatalf("connection type = %T", body.connection)
	}
	watchdog.mu.Lock()
	timeout := watchdog.timeout
	watchdog.mu.Unlock()
	if timeout != websocketCommittedStreamIdleTimeout {
		t.Fatalf("text progress timeout = %s, want %s", timeout, websocketCommittedStreamIdleTimeout)
	}
}
