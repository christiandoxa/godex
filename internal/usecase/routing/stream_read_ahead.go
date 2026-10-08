package routing

import (
	"errors"
	"io"
	"sync"
	"time"
)

const (
	defaultStreamPrecommitBytes = 768 << 10
	defaultStreamIdleTimeout    = 5 * time.Minute
	streamReadChunkSize         = 4096
)

var errStreamPrecommitLimit = errors.New("upstream SSE precommit buffer exceeded")

type streamReadResult struct {
	data []byte
	err  error
}

// streamReadAhead owns the one reader needed to enforce a precommit deadline
// without losing bytes when the stream is handed to the downstream client.
type streamReadAhead struct {
	source     io.ReadCloser
	requests   chan struct{}
	results    chan streamReadResult
	stop       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
	closeErr   error
	started    time.Time
	timeout    time.Duration
	pending    []byte
	pendingErr error
	committed  bool
	finished   bool
}

func newStreamReadAhead(source io.ReadCloser, timeout time.Duration) *streamReadAhead {
	if timeout <= 0 {
		timeout = defaultStreamIdleTimeout
	}
	body := &streamReadAhead{
		source: source, requests: make(chan struct{}), results: make(chan streamReadResult), stop: make(chan struct{}),
		done: make(chan struct{}), started: time.Now(), timeout: timeout,
	}
	go body.readAhead()
	return body
}

func (body *streamReadAhead) readAhead() {
	defer close(body.done)
	defer close(body.results)
	buffer := make([]byte, streamReadChunkSize)
	emptyReads := 0
	for {
		select {
		case <-body.requests:
		case <-body.stop:
			return
		}
		count, err := body.source.Read(buffer)
		if count > 0 {
			emptyReads = 0
		} else if err == nil {
			emptyReads++
			if emptyReads >= 100 {
				err = io.ErrNoProgress
			}
		}
		if count > 0 || err != nil {
			result := streamReadResult{data: append([]byte(nil), buffer[:count]...), err: err}
			select {
			case body.results <- result:
			case <-body.stop:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (body *streamReadAhead) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	for {
		if len(body.pending) > 0 {
			count := copy(buffer, body.pending)
			body.pending = body.pending[count:]
			return count, nil
		}
		if body.pendingErr != nil {
			err := body.pendingErr
			body.pendingErr = nil
			body.finished = true
			return 0, err
		}
		if body.finished {
			return 0, io.EOF
		}

		var timer *time.Timer
		var timeout <-chan time.Time
		if !body.committed {
			remaining := body.timeout - time.Since(body.started)
			if remaining <= 0 {
				return 0, body.timeoutBeforeCommit()
			}
			timer = time.NewTimer(remaining)
			timeout = timer.C
		}
		select {
		case body.requests <- struct{}{}:
		case <-body.stop:
			return 0, io.ErrClosedPipe
		case <-timeout:
			return 0, body.timeoutBeforeCommit()
		}
		select {
		case result, ok := <-body.results:
			if timer != nil {
				timer.Stop()
			}
			select {
			case <-body.stop:
				return 0, io.ErrClosedPipe
			default:
			}
			if !body.committed && time.Since(body.started) >= body.timeout {
				return 0, body.timeoutBeforeCommit()
			}
			if !ok {
				body.finished = true
				return 0, io.EOF
			}
			body.pending, body.pendingErr = result.data, result.err
		case <-body.stop:
			return 0, io.ErrClosedPipe
		case <-timeout:
			return 0, body.timeoutBeforeCommit()
		}
	}
}

func (body *streamReadAhead) timeoutBeforeCommit() error {
	body.finished = true
	_ = body.Close()
	return streamPrecommitTimeoutError{}
}

func (body *streamReadAhead) commit() { body.committed = true }

func (body *streamReadAhead) releaseAdmission() { releaseProfileInflight(body.source) }

func (pending *pendingResponse) commitStream() {
	if pending == nil || pending.response == nil || pending.response.Body == nil {
		return
	}
	if body, ok := pending.response.Body.(interface{ commit() }); ok {
		body.commit()
	}
}

func (body *streamReadAhead) Close() error {
	body.closeOnce.Do(func() {
		close(body.stop)
		body.closeErr = body.source.Close()
		<-body.done
	})
	return body.closeErr
}

type streamPrecommitTimeoutError struct{}

func (streamPrecommitTimeoutError) Error() string {
	return "upstream SSE stream timed out before commitment"
}
func (streamPrecommitTimeoutError) Timeout() bool   { return true }
func (streamPrecommitTimeoutError) Temporary() bool { return true }

var _ io.ReadCloser = (*streamReadAhead)(nil)
