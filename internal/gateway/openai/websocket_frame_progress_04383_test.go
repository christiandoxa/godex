package openai

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

// Tagged Prodex 0.438.3 tracks progress only when Tungstenite has assembled
// a valid upstream message. A fragment or a header trickle is not a new
// inference event and must not renew the original precommit progress clock.
func TestProdex04383IncompletePrecommitPayloadCannotRenewProgressDeadline(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	watchdog := newWebSocketReadWatchdog(local, 500*time.Millisecond)
	defer watchdog.Close()
	watchdog.mu.Lock()
	original := watchdog.deadline
	watchdog.mu.Unlock()
	// Deliberately send a valid frame header that promises a larger payload,
	// but only a tiny prefix. readWebSocketEvent must not commit any event.
	started := make(chan error, 1)
	go func() { _, err := readWebSocketEvent(watchdog); started <- err }()
	time.Sleep(60 * time.Millisecond)
	fragment := []byte{0x81, 0x18, '{', '"', 't', 'y'} // 24-byte text frame, 4 payload bytes
	if _, err := remote.Write(fragment); err != nil {
		t.Fatal(err)
	}
	// Allow the synchronous reader to finish the partial socket read; under
	// `-race` this bounded wait is much smaller than the 500ms test timeout.
	time.Sleep(22 * time.Millisecond)
	watchdog.mu.Lock()
	observed := watchdog.deadline
	watchdog.mu.Unlock()
	if !observed.Equal(original) {
		t.Fatalf("partial precommit WebSocket bytes renewed deadline by %s without a complete frame", observed.Sub(original))
	}
	select {
	case err := <-started:
		t.Fatalf("partial precommit event unexpectedly completed: %v", err)
	default:
	}
}

func TestProdex04383CompletedPrecommitFrameRenewsProgressDeadline(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	watchdog := newWebSocketReadWatchdog(local, 800*time.Millisecond)
	defer watchdog.Close()
	watchdog.mu.Lock()
	original := watchdog.deadline
	watchdog.mu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := readWebSocketEvent(watchdog); done <- err }()
	time.Sleep(60 * time.Millisecond)
	var encoded bytes.Buffer
	payload := []byte(`{"type":"response.created","response":{"id":"synthetic"}}`)
	if err := websocketframe.WriteFrame(&encoded, 1, payload, false); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.Write(encoded.Bytes()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("complete WebSocket event stalled")
	}
	watchdog.mu.Lock()
	observed := watchdog.deadline
	watchdog.mu.Unlock()
	if observed.Sub(original) < 45*time.Millisecond {
		t.Fatalf("complete upstream frame did not refresh idle progress; advance=%s", observed.Sub(original))
	}
}

func TestProdex04383PartialPrecommitFrameStillTimesOut(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	watchdog := newWebSocketReadWatchdog(local, 85*time.Millisecond)
	defer watchdog.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := readWebSocketEvent(watchdog); done <- err }()
	if _, err := remote.Write([]byte{0x81, 0x10, '{'}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || errors.Is(err, context.Canceled) {
			t.Fatalf("partial frame unexpectedly committed or canceled: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("incomplete frame trickle blocked past bounded watchdog")
	}
}

func TestProdex04383FragmentedMessageAdvancesProgressOnlyAfterFinalContinuation(t *testing.T) {
	local, remote := net.Pipe()
	defer remote.Close()
	watcher := newWebSocketReadWatchdog(local, 850*time.Millisecond)
	defer watcher.Close()
	watcher.mu.Lock()
	original := watcher.deadline
	watcher.mu.Unlock()
	received := make(chan websocketEvent, 1)
	fail := make(chan error, 1)
	go func() {
		event, err := readWebSocketEvent(watcher)
		if err != nil {
			fail <- err
			return
		}
		received <- event
	}()
	time.Sleep(60 * time.Millisecond)
	full := []byte(`{"type":"response.completed","response":{"id":"partial-04383"}}`)
	pivot := len(full) / 2
	// FIN=0 text, followed by FIN=1 continuation. Tungstenite only emits
	// the assembled logical text event after the second frame arrives.
	fragment := append([]byte{0x01, byte(pivot)}, full[:pivot]...)
	if _, err := remote.Write(fragment); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	watcher.mu.Lock()
	partial := watcher.deadline
	watcher.mu.Unlock()
	if !partial.Equal(original) {
		t.Fatalf("unfinished continuation extended idle by %s", partial.Sub(original))
	}
	select {
	case event := <-received:
		t.Fatalf("unfinished message was committed: %q", event.kind)
	case err := <-fail:
		t.Fatalf("unfinished message failed: %v", err)
	default:
	}
	final := append([]byte{0x80, byte(len(full) - pivot)}, full[pivot:]...)
	if _, err := remote.Write(final); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-received:
		if event.kind != "response.completed" || string(event.payload) != string(full) || !event.terminal {
			t.Fatalf("completed fragmented event changed: kind=%q payload=%q terminal=%v", event.kind, event.payload, event.terminal)
		}
	case err := <-fail:
		t.Fatalf("completed fragmented frame failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("final continuation never completed")
	}
	watcher.mu.Lock()
	completed := watcher.deadline
	watcher.mu.Unlock()
	if completed.Sub(original) < 40*time.Millisecond {
		t.Fatalf("complete message did not record progress: %s", completed.Sub(original))
	}
}
