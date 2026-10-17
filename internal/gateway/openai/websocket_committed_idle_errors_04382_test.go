package openai

import (
	"errors"
	"net"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04382CommittedIdleTimeoutIsTypedWithoutReplaying(t *testing.T) {
	for _, tc := range []struct {
		name      string
		committed bool
		wantTyped bool
	}{
		{name: "committed model output", committed: true, wantTyped: true},
		{name: "precommit has separate timeout ownership", wantTyped: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, peer := net.Pipe()
			defer peer.Close()
			timeout := 60 * time.Millisecond
			watchdog := newWebSocketReadWatchdog(client, timeout)
			defer watchdog.Close()
			if tc.committed {
				watchdog.markCommittedIdle04382(timeout)
			}
			errorChannel := make(chan error, 1)
			go func() { _, err := watchdog.Read(make([]byte, 8)); errorChannel <- err }()
			select {
			case err := <-errorChannel:
				if err == nil || errors.Is(err, proxymodel.ErrWebSocketCommittedIdleTimeout) != tc.wantTyped {
					t.Fatalf("expired socket error=%v want committed sentinel=%t", err, tc.wantTyped)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("watchdog did not unblock read")
			}
		})
	}
}

func TestProdex04382RecycledSocketClearsCommittedIdleOwnership(t *testing.T) {
	client, peer := net.Pipe()
	defer peer.Close()
	watchdog := newWebSocketReadWatchdog(client, 2*time.Second)
	defer watchdog.Close()
	watchdog.markCommittedIdle04382(5 * time.Second)
	watchdog.pause()
	reused := newWebSocketReadWatchdog(watchdog, 65*time.Millisecond)
	if reused != watchdog {
		t.Fatal("lost upstream session identity when recycling socket")
	}
	watchdog.mu.Lock()
	committed := watchdog.committed
	paused := watchdog.paused
	watchdog.mu.Unlock()
	if committed || paused {
		t.Fatalf("recycled socket stayed committed=%t or paused=%t", committed, paused)
	}
	errCh := make(chan error, 1)
	go func() { _, err := reused.Read(make([]byte, 8)); errCh <- err }()
	select {
	case err := <-errCh:
		if err == nil || errors.Is(err, proxymodel.ErrWebSocketCommittedIdleTimeout) {
			t.Fatalf("reused PRECOMMIT read was mislabeled as committed idle: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reused precommit watchdog did not expire")
	}
}
