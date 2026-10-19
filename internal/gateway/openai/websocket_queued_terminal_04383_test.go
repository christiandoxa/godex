package openai

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// Prodex 0.438.3 calls the nonblocking frame reader BEFORE checking its
// upstream idle clock. The first terminal frame may already be in TCP's
// receive queue while synchronous commit/persistence consumes >idle time.
// A queued completed/failed frame must not be invented as a timeout.
func TestProdex04383QueuedTerminalBeatsExpiredCommittedIdle(t *testing.T) {
	cases := []struct {
		name string
		body string
		kind string
	}{
		{"completed", `{"type":"response.completed","response":{"id":"queued-1"}}`, "response.completed"},
		{"failed", `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`, "response.failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				server, err := listener.Accept()
				if err == nil {
					accepted <- server
				}
			}()
			client, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			server := <-accepted
			defer server.Close()
			const idle = 90 * time.Millisecond
			watchdog := newWebSocketReadWatchdog(client, idle)
			watchdog.markCommittedIdle04382(idle)
			defer watchdog.Close()
			if err := websocketframe.WriteFrame(server, 1, []byte(tc.body), false); err != nil {
				t.Fatal(err)
			}
			// The upstream frame is now committed to the OS socket queue.
			// Simulate a separate synchronous persistence/dispatch pause.
			time.Sleep(160 * time.Millisecond)
			event, err := readWebSocketEvent(watchdog)
			if err != nil {
				t.Fatalf("already-buffered %s was mistaken for upstream idle: %v", tc.kind, err)
			}
			if event.kind != tc.kind || string(event.payload) != tc.body {
				t.Fatalf("upstream terminal mutated: kind=%q payload=%s want=%s", event.kind, event.payload, tc.body)
			}
		})
	}
}

func TestProdex04383GenuinelySilentCommittedStreamStillTimesOut(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	watchdog := newWebSocketReadWatchdog(left, 65*time.Millisecond)
	watchdog.markCommittedIdle04382(65 * time.Millisecond)
	defer watchdog.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := watchdog.Read(make([]byte, 8)); result <- err }()
	select {
	case err := <-result:
		if !errors.Is(err, proxymodel.ErrWebSocketCommittedIdleTimeout) {
			t.Fatalf("silent committed stream lost configured timeout: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("genuinely silent upstream did not expire")
	}
}

// When no data was queued at the timer boundary, the deferred readiness
// check must remain bounded. Otherwise a quiet connection can outlive its
// configured committed idle forever waiting for the next application read.
func TestProdex04383ExpiredQuietSocketWithoutActiveReaderStillTimesOut(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		remote, err := listener.Accept()
		if err == nil {
			accepted <- remote
		}
	}()
	local, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	remote := <-accepted
	defer remote.Close()
	watcher := newWebSocketReadWatchdog(local, 65*time.Millisecond)
	watcher.markCommittedIdle04382(65 * time.Millisecond)
	defer watcher.Close()
	time.Sleep(135 * time.Millisecond)
	started := time.Now()
	_, err = watcher.Read(make([]byte, 16))
	if !errors.Is(err, proxymodel.ErrWebSocketCommittedIdleTimeout) {
		t.Fatalf("idle socket without queued data escaped committed timeout: %v", err)
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatalf("post-expiry readiness probe blocked instead of failing fast")
	}
}

// The timer's short queued-readiness probe is allowed to preserve one
// already-buffered byte, but it cannot turn a FIN into a model response.
func TestProdex04383ExpiredQuietSocketBeforeCommitIsNotCommittedIdle(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	watcher := newWebSocketReadWatchdog(left, 40*time.Millisecond)
	defer watcher.Close()
	time.Sleep(90 * time.Millisecond)
	_, err := watcher.Read(make([]byte, 16))
	if err == nil || errors.Is(err, proxymodel.ErrWebSocketCommittedIdleTimeout) {
		t.Fatalf("precommit quiet socket mislabeled as committed idle: %v", err)
	}
}
