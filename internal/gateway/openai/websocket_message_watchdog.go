package openai

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
	"net"
	"sync"
	"time"
)

type websocketReadWatchdog struct {
	connection io.ReadWriteCloser

	mu        sync.Mutex
	timer     *time.Timer
	timeout   time.Duration
	deadline  time.Time
	paused    bool
	closed    bool
	expired   bool
	committed bool
	reading   bool
}

func newWebSocketReadWatchdog(connection io.ReadWriteCloser, timeout time.Duration) *websocketReadWatchdog {
	if existing, ok := connection.(*websocketReadWatchdog); ok {
		existing.setTimeout(timeout)
		return existing
	}
	watchdog := &websocketReadWatchdog{connection: connection}
	watchdog.setTimeout(timeout)
	return watchdog
}

type websocketDeadlineSetter04383 interface {
	SetReadDeadline(time.Time) error
}

// A queued TCP/TLS frame must be checked BEFORE classifying the connection
// as idle. Prodex 0.438.3 explicitly drains an already-buffered terminal
// frame before applying its timeout; a Go timer closing an unread socket
// discarded response.completed/failed while local delivery was busy.
func (watchdog *websocketReadWatchdog) Read(buffer []byte) (int, error) {
	watchdog.mu.Lock()
	if watchdog.closed {
		committed := watchdog.expired && watchdog.committed
		watchdog.mu.Unlock()
		if committed {
			return 0, proxymodel.ErrWebSocketCommittedIdleTimeout
		}
		return 0, net.ErrClosed
	}
	// A timer that expired when NO read was pending leaves the socket live
	// long enough for one bounded, readiness-like buffered-data probe.
	// SetReadDeadline only if supported (net.Conn and tls.Conn); never use
	// blocking SO_RCVTIMEO on Windows and never update the logical watchdog.
	deferred := watchdog.expired
	deadlineConn, hasDeadline := watchdog.connection.(websocketDeadlineSetter04383)
	if deferred && hasDeadline {
		if err := deadlineConn.SetReadDeadline(time.Now().Add(5 * time.Millisecond)); err != nil {
			watchdog.closed = true
			watchdog.mu.Unlock()
			_ = watchdog.connection.Close()
			return 0, err
		}
	}
	watchdog.reading = true
	watchdog.mu.Unlock()
	count, err := watchdog.connection.Read(buffer)
	if deferred && hasDeadline {
		_ = deadlineConn.SetReadDeadline(time.Time{})
	}
	watchdog.mu.Lock()
	watchdog.reading = false
	if count > 0 && deferred && !watchdog.closed {
		watchdog.expired = false
	}
	expired := watchdog.expired
	committed := watchdog.committed
	watchdog.mu.Unlock()
	if count > 0 {
		watchdog.noteProgress()
		return count, err
	}
	if deferred {
		watchdog.mu.Lock()
		if !watchdog.closed {
			watchdog.closed = true
		}
		watchdog.mu.Unlock()
		_ = watchdog.connection.Close()
		if committed {
			return 0, proxymodel.ErrWebSocketCommittedIdleTimeout
		}
		if err != nil {
			return 0, err
		}
		return 0, net.ErrClosed
	}
	if err != nil && expired && committed {
		return 0, proxymodel.ErrWebSocketCommittedIdleTimeout
	}
	return count, err
}

func (watchdog *websocketReadWatchdog) Write(buffer []byte) (int, error) {
	return watchdog.connection.Write(buffer)
}

func (watchdog *websocketReadWatchdog) Close() error {
	watchdog.mu.Lock()
	if watchdog.closed {
		watchdog.mu.Unlock()
		return nil
	}
	watchdog.closed = true
	if watchdog.timer != nil {
		watchdog.timer.Stop()
	}
	watchdog.mu.Unlock()
	return watchdog.connection.Close()
}

func (watchdog *websocketReadWatchdog) setTimeout(timeout time.Duration) {
	watchdog.setTimeoutPhase04382(timeout, false)
}

// Reused upstream sockets move back to PRECOMMIT before the next message.
// Updating the timeout and its ownership must be atomic so an old committed
// timer cannot relabel the new request's precommit cancellation/expiry.
func (watchdog *websocketReadWatchdog) markCommittedIdle04382(timeout time.Duration) {
	watchdog.setTimeoutPhase04382(timeout, true)
}

func (watchdog *websocketReadWatchdog) setTimeoutPhase04382(timeout time.Duration, committed bool) {
	if timeout <= 0 {
		return
	}
	watchdog.mu.Lock()
	if watchdog.closed {
		watchdog.mu.Unlock()
		return
	}
	watchdog.committed = committed
	watchdog.expired = false
	watchdog.timeout = timeout
	watchdog.deadline = time.Now().Add(timeout)
	watchdog.paused = false
	if watchdog.timer == nil {
		watchdog.timer = time.AfterFunc(timeout, watchdog.expire)
	} else {
		watchdog.timer.Stop()
		watchdog.timer.Reset(timeout)
	}
	watchdog.mu.Unlock()
}

func (watchdog *websocketReadWatchdog) noteProgress() {
	watchdog.mu.Lock()
	if watchdog.closed || watchdog.paused || watchdog.timeout <= 0 {
		watchdog.mu.Unlock()
		return
	}
	timeout := watchdog.timeout
	watchdog.deadline = time.Now().Add(timeout)
	if watchdog.timer == nil {
		watchdog.timer = time.AfterFunc(timeout, watchdog.expire)
	} else {
		watchdog.timer.Stop()
		watchdog.timer.Reset(timeout)
	}
	watchdog.mu.Unlock()
}

func (watchdog *websocketReadWatchdog) pause() {
	watchdog.mu.Lock()
	watchdog.paused = true
	if watchdog.timer != nil {
		watchdog.timer.Stop()
	}
	watchdog.mu.Unlock()
}

func (watchdog *websocketReadWatchdog) expire() {
	watchdog.mu.Lock()
	if watchdog.closed || watchdog.paused {
		watchdog.mu.Unlock()
		return
	}
	remaining := time.Until(watchdog.deadline)
	if remaining > 0 {
		watchdog.timer.Reset(remaining)
		watchdog.mu.Unlock()
		return
	}
	watchdog.expired = true
	// If application-side state persistence delayed its NEXT read while a
	// terminal frame arrived in the kernel buffer, keep the connection long
	// enough to perform a single nonblocking-style read at that boundary.
	// An ACTIVE blocked read still expires immediately, preserving genuine
	// silence semantics. Non-deadline transports retain legacy closure.
	_, canProbe := watchdog.connection.(websocketDeadlineSetter04383)
	if !watchdog.reading && canProbe {
		watchdog.mu.Unlock()
		return
	}
	watchdog.closed = true
	watchdog.mu.Unlock()
	_ = watchdog.connection.Close()
}
