package openai

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged Prodex 0.438.2 response_tracking/commit.rs restores the actual
// configured `stream_idle_timeout_ms` after commitment. Neither the fixed
// precommit progress timeout nor the request-wide watchdog can substitute
// for this separately-owned committed stream idle timeout.
func TestProdex04382WebSocketCommittedOutputUsesConfiguredStreamIdle(t *testing.T) {
	for _, tc := range []struct {
		name             string
		configured, want time.Duration
	}{
		{name: "default published 300 seconds", want: 300 * time.Second},
		{name: "stream idle 250 milliseconds", configured: 250 * time.Millisecond, want: 250 * time.Millisecond},
		{name: "stream idle two seconds", configured: 2 * time.Second, want: 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"type":"response.output_text.delta","delta":"visible"}`)
			server := newWebSocketMessageParityServer(t, nil, func(conn net.Conn, reader *bufio.Reader, _ *http.Request) {
				if _, err := readWebSocketParityRequest(reader); err != nil {
					t.Errorf("read upstream request: %v", err)
					return
				}
				if err := writeWebSocketParityEvent(conn, payload); err != nil {
					t.Errorf("send output delta: %v", err)
					return
				}
				// Hold after a visible model delta to inspect the committed
				// socket state without resetting the precommit deadline.
				_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				_, _ = conn.Read(make([]byte, 1))
			})
			defer server.Close()
			transport, err := NewTransportWithCommittedWebSocketIdle04382(server.URL, nil, websocketAuth{}, tc.configured)
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			result, err := transport.ExecuteWebSocketMessage(context.Background(),
				websocketParityRequest(`{"type":"response.create"}`, 438201, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
				proxymodel.Account{ID: "synthetic", Home: "synthetic-home"})
			if err != nil {
				t.Fatal(err)
			}
			defer result.Body.Close()
			if !result.FirstEventCommitted {
				t.Fatalf("visible delta did not commit the model turn: %+v", result)
			}
			body, ok := result.Body.(*websocketResponseBody)
			if !ok {
				t.Fatalf("unexpected response body %T", result.Body)
			}
			watchdog, ok := body.connection.(*websocketReadWatchdog)
			if !ok {
				t.Fatalf("unexpected committed connection %T", body.connection)
			}
			watchdog.mu.Lock()
			got := watchdog.timeout
			watchdog.mu.Unlock()
			if got != tc.want {
				t.Fatalf("post-commit stream idle=%s, want %s (precommit must remain independently bounded)", got, tc.want)
			}
		})
	}
}
