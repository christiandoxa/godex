package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

const (
	defaultSSELookaheadTimeout  = time.Second
	defaultSSEStreamIdleTimeout = 5 * time.Minute
	sseLookaheadMaxBytes        = 8 << 10
)

type anthropicReadResult struct {
	data []byte
	err  error
}

// One reader carries inspected bytes and its live read into the committed stream.
type anthropicReadAhead struct {
	source      io.ReadCloser
	results     chan anthropicReadResult
	stop        chan struct{}
	closeOnce   sync.Once
	closeErr    error
	idleTimeout time.Duration
}

type anthropicLookaheadBody struct {
	ahead      *anthropicReadAhead
	prefix     []byte
	buffered   []byte
	pendingErr error
	terminal   bool
}

func peekAnthropicFirstEvent(ctx context.Context, body io.ReadCloser, timeout, idleTimeout time.Duration) (io.ReadCloser, []byte, error) {
	if timeout <= 0 {
		timeout = defaultSSELookaheadTimeout
	}
	if idleTimeout <= 0 {
		idleTimeout = defaultSSEStreamIdleTimeout
	}
	ahead := newAnthropicReadAhead(body, idleTimeout)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	decoder := sse.NewDecoder(nativeMessagesMaxBytes)
	prefix := make([]byte, 0, sseLookaheadMaxBytes)
	for len(prefix) < sseLookaheadMaxBytes {
		if err := ctx.Err(); err != nil {
			_ = ahead.Close()
			return nil, nil, err
		}
		select {
		case result, ok := <-ahead.results:
			if err := ctx.Err(); err != nil {
				_ = ahead.Close()
				return nil, nil, err
			}
			if !ok {
				return &anthropicLookaheadBody{ahead: ahead, prefix: prefix}, nil, nil
			}
			remaining := sseLookaheadMaxBytes - len(prefix)
			inspect := result.data
			if len(inspect) > remaining {
				inspect = inspect[:remaining]
			}
			prefix = append(prefix, inspect...)
			suffix := result.data[len(inspect):]
			for _, event := range decoder.Feed(inspect) {
				if !anthropicStreamPing(event) {
					return &anthropicLookaheadBody{
						ahead: ahead, prefix: prefix, buffered: suffix, pendingErr: result.err,
					}, event, nil
				}
			}
			if len(prefix) == sseLookaheadMaxBytes || len(suffix) > 0 {
				return &anthropicLookaheadBody{
					ahead: ahead, prefix: prefix, buffered: suffix, pendingErr: result.err,
				}, nil, nil
			}
			if result.err != nil {
				return &anthropicLookaheadBody{ahead: ahead, prefix: prefix, pendingErr: result.err}, nil, result.err
			}
		case <-timer.C:
			return &anthropicLookaheadBody{ahead: ahead, prefix: prefix}, nil, nil
		case <-ctx.Done():
			_ = ahead.Close()
			return nil, nil, ctx.Err()
		}
	}
	return &anthropicLookaheadBody{ahead: ahead, prefix: prefix}, nil, nil
}

func newAnthropicReadAhead(source io.ReadCloser, idleTimeout time.Duration) *anthropicReadAhead {
	if idleTimeout <= 0 {
		idleTimeout = defaultSSEStreamIdleTimeout
	}
	ahead := &anthropicReadAhead{
		source: source, results: make(chan anthropicReadResult), stop: make(chan struct{}), idleTimeout: idleTimeout,
	}
	go func() {
		defer close(ahead.results)
		buffer := make([]byte, sseLookaheadMaxBytes)
		for {
			read, err := source.Read(buffer)
			result := anthropicReadResult{data: append([]byte(nil), buffer[:read]...), err: err}
			select {
			case ahead.results <- result:
			case <-ahead.stop:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return ahead
}

func (body *anthropicLookaheadBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	for {
		if len(body.prefix) > 0 {
			read := copy(buffer, body.prefix)
			body.prefix = body.prefix[read:]
			return read, nil
		}
		if len(body.buffered) > 0 {
			read := copy(buffer, body.buffered)
			body.buffered = body.buffered[read:]
			if len(body.buffered) == 0 && body.pendingErr != nil {
				body.terminal = true
			}
			return read, nil
		}
		if body.terminal {
			if body.pendingErr == nil {
				return 0, io.EOF
			}
			return 0, body.pendingErr
		}
		if body.pendingErr != nil {
			body.terminal = true
			return 0, body.pendingErr
		}
		timer := time.NewTimer(body.ahead.idleTimeout)
		select {
		case result, ok := <-body.ahead.results:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if !ok {
				return 0, io.EOF
			}
			body.buffered, body.pendingErr = result.data, result.err
		case <-timer.C:
			_ = body.ahead.Close()
			body.terminal = true
			body.pendingErr = &streamIdleTimeoutError{}
			return 0, body.pendingErr
		}
	}
}

type streamIdleTimeoutError struct{}

func (*streamIdleTimeoutError) Error() string   { return "runtime upstream stream idle timed out" }
func (*streamIdleTimeoutError) Timeout() bool   { return true }
func (*streamIdleTimeoutError) Temporary() bool { return true }

func (ahead *anthropicReadAhead) Close() error {
	ahead.closeOnce.Do(func() {
		close(ahead.stop)
		ahead.closeErr = ahead.source.Close()
	})
	return ahead.closeErr
}

func (body *anthropicLookaheadBody) Close() error { return body.ahead.Close() }

func anthropicStreamPing(data []byte) bool {
	var event map[string]any
	return json.Unmarshal(data, &event) == nil && event["type"] == "ping"
}

var _ io.ReadCloser = (*anthropicLookaheadBody)(nil)
