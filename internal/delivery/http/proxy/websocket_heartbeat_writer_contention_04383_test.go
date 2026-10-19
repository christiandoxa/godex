package proxy

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

// A large model-data/terminal frame owns writer.mu while it flushes to the
// client. A heartbeat is advisory transport control, not application output:
// it must NEVER block teardown behind that frame's writer lock. This is
// especially important when the peer stalls with the weighted profile live.
func TestProdex04383HeartbeatSkipsContendedModelWriter(t *testing.T) {
	client, peer := net.Pipe()
	defer peer.Close()
	defer client.Close()
	tunnel := &responsesWebSocketTunnel{
		client: client, toClient: &websocketFrameWriter{writer: bufio.NewWriter(client)},
	}
	tunnel.toClient.mu.Lock()
	done := make(chan error, 1)
	go func() { done <- tunnel.sendHeartbeat04383() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("contention should skip an advisory Ping, not fail the model turn: %v", err)
		}
	case <-time.After(120 * time.Millisecond):
		tunnel.toClient.mu.Unlock()
		<-done
		t.Fatal("transport Ping blocked behind active model output writer")
	}
	tunnel.toClient.mu.Unlock()
}

func TestProdex04383HeartbeatShutdownDoesNotWaitForBlockedClientWriter(t *testing.T) {
	client, peer := net.Pipe()
	defer peer.Close()
	defer client.Close()
	tunnel := &responsesWebSocketTunnel{
		client: client, toClient: &websocketFrameWriter{writer: bufio.NewWriter(client)},
	}
	tunnel.toClient.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := tunnel.startHeartbeat04383(ctx)
	// Permit one heartbeat tick while the model-frame writer is busy.
	time.Sleep(websocketClientHeartbeatInterval04383 + 80*time.Millisecond)
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(200 * time.Millisecond):
		// Release the lock before reporting failure to avoid leaking the
		// worker goroutine or hanging the Go test after a red regression.
		tunnel.toClient.mu.Unlock()
		<-stopped
		t.Fatal("completed WebSocket shutdown blocked on advisory Ping writer lock")
	}
	tunnel.toClient.mu.Unlock()
}
