package openai

import (
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"io"
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

func (watchdog *websocketReadWatchdog) Read(buffer []byte) (int, error) {
	count, err := watchdog.connection.Read(buffer)
	if count > 0 {
		watchdog.noteProgress()
	}
	if count == 0 && err != nil {
		watchdog.mu.Lock()
		committedExpiry := watchdog.expired && watchdog.committed
		watchdog.mu.Unlock()
		if committedExpiry {
			return 0, proxymodel.ErrWebSocketCommittedIdleTimeout
		}
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
	watchdog.closed = true
	watchdog.expired = true
	watchdog.mu.Unlock()
	_ = watchdog.connection.Close()
}
