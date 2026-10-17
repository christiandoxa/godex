package openai

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Tagged Prodex 0.438.2 response_tracking/precommit.rs promotes fresh
// bounded holds at 8KiB, then invokes the SAME commit.rs timeout policy as
// a model output delta: the upstream idle clock switches to the configured
// committed-stream timeout. Below lookahead, hidden bytes never promote.
func TestProdex04382LookaheadPromotedWebSocketUsesConfiguredCommittedIdle(t *testing.T) {
	for _, tc := range []struct {
		name             string
		configured, want time.Duration
	}{
		{name: "published default", want: 300 * time.Second},
		{name: "configured stream idle", configured: 2 * time.Second, want: 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arrived := make(chan struct{})
			server := newWebSocketMessageParityServer(t, nil, func(conn net.Conn, reader *bufio.Reader, _ *http.Request) {
				if _, err := readWebSocketParityRequest(reader); err != nil {
					t.Errorf("request: %v", err)
					return
				}
				payload := []byte(`{"type":"response.in_progress","padding":"` +
					strings.Repeat("x", websocketPrecommitLookaheadBytes) + `"}`)
				if err := writeWebSocketParityEvent(conn, payload); err != nil {
					t.Errorf("hold frame: %v", err)
					return
				}
				close(arrived)
				_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
				_, _ = conn.Read(make([]byte, 1))
			})
			defer server.Close()
			transport, err := NewTransportWithCommittedWebSocketIdle04382(server.URL, nil, websocketAuth{}, tc.configured)
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			response, err := transport.ExecuteWebSocketMessage(ctx,
				websocketParityRequest(`{"type":"response.create"}`, 438202, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
				proxymodel.Account{ID: "synthetic", Home: "synthetic-home"})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			<-arrived
			if !response.FirstEventCommitted {
				t.Fatal("8KiB promoted hold did not commit")
			}
			body, ok := response.Body.(*websocketResponseBody)
			if !ok {
				t.Fatalf("response body %T", response.Body)
			}
			watchdog, ok := body.connection.(*websocketReadWatchdog)
			if !ok {
				t.Fatalf("watchdog type %T", body.connection)
			}
			watchdog.mu.Lock()
			timeout, committed := watchdog.timeout, watchdog.committed
			watchdog.mu.Unlock()
			if !committed || timeout != tc.want {
				t.Fatalf("lookahead-committed upstream idle=%s committed=%t, want %s/true", timeout, committed, tc.want)
			}
		})
	}
}
