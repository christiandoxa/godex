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

// Prodex 0.437.9 treats a 5xx status on a Responses *error* as a
// failure signal, not a status-shaped attribute on model output or a heartbeat.
func TestProdex04379WebSocketNonErrorStatusPreservesOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		first  string
		second string
		third  string
	}{
		{
			name:   "visible output with status-like metadata",
			first:  `{"type":"response.output_text.delta","status":503,"delta":"visible"}`,
			second: `{"type":"response.completed","response":{"id":"resp-output"}}`,
		},
		{
			name:   "heartbeat with status-like metadata",
			first:  `{"type":"keepalive","status":503}`,
			second: `{"type":"response.output_text.delta","delta":"visible"}`,
			third:  `{"type":"response.completed","response":{"id":"resp-heartbeat"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, second, third := []byte(tc.first), []byte(tc.second), []byte(tc.third)
			if got := websocketRetryFailureCode(first); got != "" {
				t.Fatalf("non-error event was classified retryable: %q", got)
			}
			server := newWebSocketMessageParityServer(t, nil,
				func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
					if _, err := readWebSocketParityRequest(reader); err != nil {
						t.Errorf("upstream request: %v", err)
						return
					}
					if err := writeWebSocketParityEvent(connection, first); err != nil {
						t.Errorf("write first event: %v", err)
						return
					}
					if err := writeWebSocketParityEvent(connection, second); err != nil {
						t.Errorf("write subsequent output: %v", err)
						return
					}
					if len(third) > 0 {
						_ = writeWebSocketParityEvent(connection, third)
					}
				},
			)
			defer server.Close()
			transport := newWebSocketParityTransport(t, server)
			response, err := transport.ExecuteWebSocketMessage(
				context.Background(),
				websocketParityRequest(`{"type":"response.create"}`, 439,
					proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
				proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
			)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if !response.FirstEventCommitted || response.PrecommitFailure != nil {
				t.Fatalf("output after non-error 503 must commit, not retry: %+v", response)
			}
			wire, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			frames := [][]byte{first, second}
			if len(third) > 0 {
				frames = append(frames, third)
			}
			if want := websocketParityFrames(t, frames...); !bytes.Equal(wire, want) {
				t.Fatalf("committed output bytes = %q, want %q", wire, want)
			}
		})
	}
}

func TestProdex04379WebSocketTypedErrorsStillRetryWithStatus(t *testing.T) {
	for _, tc := range []struct{ payload, code string }{
		{`{"type":"error","status":503,"error":{"code":"server_is_overloaded"}}`, "server_is_overloaded"},
		{`{"type":"response.failed","status":429,"response":{"error":{"code":"rate_limit_exceeded"}}}`, "rate_limit_exceeded"},
		{`{"status":503}`, "server_is_overloaded"},
	} {
		if got := websocketRetryFailureCode([]byte(tc.payload)); got != tc.code {
			t.Fatalf("typed retry event %s: got %q, want %q", tc.payload, got, tc.code)
		}
	}
}
