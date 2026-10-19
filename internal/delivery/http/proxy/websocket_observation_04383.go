package proxy

import (
	"context"
	"errors"
	"io"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketObservedReader04383 struct {
	io.Reader
	event   *proxymodel.WebSocketEvent
	seen    uint64
	observe func([]byte) error
}

func (reader *websocketObservedReader04383) Read(buffer []byte) (int, error) {
	count, err := reader.Reader.Read(buffer)
	if reader.event.Sequence != reader.seen {
		reader.seen = reader.event.Sequence
		if observeErr := reader.observe(reader.event.Payload); observeErr != nil {
			return 0, observeErr
		}
	}
	return count, err
}

func (reader *websocketObservedReader04383) Write(payload []byte) (int, error) {
	if writer, ok := reader.Reader.(io.Writer); ok {
		return writer.Write(payload)
	}
	return 0, errors.New("upstream websocket connection is not duplex")
}

func (proxy *Proxy) websocketResponseReader04383(ctx context.Context, result proxymodel.Forwarded) io.Reader {
	if result.Failed || result.Response.WebSocketEvent == nil {
		return result.Response.Body
	}
	return &websocketObservedReader04383{
		Reader: result.Response.Body, event: result.Response.WebSocketEvent,
		observe: func(payload []byte) error {
			return proxy.router.Observe(ctx, result.AccountID, nil, payload, false, result.ProviderKind)
		},
	}
}
