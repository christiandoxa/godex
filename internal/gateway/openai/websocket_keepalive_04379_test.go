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

// A Responses heartbeat is upstream liveness, not committed model output.
func TestProdex04379WebSocketKeepaliveRemainsPrecommit(t *testing.T) {
	heartbeat := []byte(`{"type":"keepalive"}`)
	failure := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`)
	if !isWebSocketPrecommitHold("keepalive") {
		t.Fatal("keepalive classified as committed output")
	}
	server := newWebSocketMessageParityServer(t, nil, func(connection net.Conn, reader *bufio.Reader, _ *http.Request) {
		if _, err := readWebSocketParityRequest(reader); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err := writeWebSocketParityEvent(connection, heartbeat); err != nil {
			t.Errorf("write heartbeat: %v", err)
			return
		}
		if err := writeWebSocketParityEvent(connection, failure); err != nil {
			t.Errorf("write failure: %v", err)
		}
	})
	defer server.Close()
	transport := newWebSocketParityTransport(t, server)
	response, err := transport.ExecuteWebSocketMessage(context.Background(),
		websocketParityRequest(`{"type":"response.create"}`, 179, proxymodel.WebSocketPolicy{PromoteCommittedProfile: true}),
		proxymodel.Account{ID: "profile-a", Home: "synthetic-home"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.FirstEventCommitted || response.PrecommitFailure == nil ||
		response.PrecommitFailure.Code != "server_is_overloaded" {
		t.Fatalf("heartbeat prematurely committed response: %#v", response)
	}
	wire, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if want := websocketParityFrames(t, failure); !bytes.Equal(wire, want) {
		t.Fatalf("retryable failure leaked held heartbeat: got %d bytes want %d", len(wire), len(want))
	}
}

func TestProdex04379WebSocketOutputAfterKeepaliveStillCommits(t *testing.T) {
	for _, kind := range []string{"response.output_text.delta", "response.reasoning_summary_text.delta", "response.function_call_arguments.delta"} {
		if isWebSocketPrecommitHold(kind) {
			t.Fatalf("actual output %s was marked as precommit", kind)
		}
	}
}
