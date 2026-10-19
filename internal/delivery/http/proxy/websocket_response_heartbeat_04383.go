package proxy

import (
	"context"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

const (
	websocketClientHeartbeatInterval04383   = time.Second
	websocketClientHeartbeatWriteLimit04383 = time.Second
)

// Prodex 0.438.3 sends transport-only empty WebSocket Pings while a
// Responses turn is safely waiting for upstream input, including held
// precommit metadata. Pings keep the LOCAL client alive; they are never
// model output, never renew an upstream progress timer, never extend the
// absolute request deadline, and never authorize a cross-profile replay.
// Realtime duplex has a separate committed-session lifecycle and does not
// use this Responses-only heartbeat worker.
func (tunnel *responsesWebSocketTunnel) startHeartbeat04383(ctx context.Context) func() {
	stopped := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(websocketClientHeartbeatInterval04383)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopped:
				return
			case <-ticker.C:
				select {
				case <-ctx.Done():
					return
				case <-stopped:
					return
				default:
				}
				if err := tunnel.sendHeartbeat04383(); err != nil {
					// A client that stopped receiving cannot hold a
					// profile permit or an upstream read indefinitely.
					tunnel.close()
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stopped); <-finished }) }
}

func (tunnel *responsesWebSocketTunnel) sendHeartbeat04383() error {
	writer := tunnel.toClient
	writer.mu.Lock()
	defer writer.mu.Unlock()
	// Hold writer.mu throughout deadline installation and restoration:
	// otherwise a concurrent model/close frame could inherit the Ping's
	// bounded write deadline, or interleave frame bytes on a shared socket.
	if err := tunnel.client.SetWriteDeadline(time.Now().Add(websocketClientHeartbeatWriteLimit04383)); err != nil {
		return err
	}
	defer tunnel.client.SetWriteDeadline(time.Time{})
	if err := websocketframe.WriteFrame(writer.writer, 9, nil, false); err != nil {
		return err
	}
	return writer.writer.Flush()
}
