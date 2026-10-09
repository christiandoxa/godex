package openai

import (
	"context"
	"io"
	"sync"
	"time"
)

type websocketHandshakeBody struct {
	io.ReadCloser
	duplex io.ReadWriteCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (body *websocketHandshakeBody) Read(buffer []byte) (int, error) {
	count, err := body.ReadCloser.Read(buffer)
	if err != nil {
		body.cancelOnce()
	}
	return count, err
}

func (body *websocketHandshakeBody) Write(buffer []byte) (int, error) {
	if body.duplex == nil {
		return 0, io.ErrClosedPipe
	}
	count, err := body.duplex.Write(buffer)
	if err != nil {
		body.cancelOnce()
	}
	return count, err
}

func (body *websocketHandshakeBody) Close() error {
	err := body.ReadCloser.Close()
	body.cancelOnce()
	return err
}

func (body *websocketHandshakeBody) cancelOnce() {
	body.once.Do(body.cancel)
}

func websocketHandshakeContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc, *time.Timer) {
	ctx, cancel := context.WithCancel(parent)
	return ctx, cancel, time.AfterFunc(timeout, cancel)
}
