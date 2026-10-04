package proxy

import (
	"context"
	"io"
	"net"
	"sync"
)

type responsesWebSocketTunnel struct {
	client   net.Conn
	upstream io.Closer
	toClient *websocketFrameWriter
	cancel   context.CancelFunc

	once   sync.Once
	mu     sync.Mutex
	closed bool
}

func (tunnel *responsesWebSocketTunnel) close() {
	tunnel.once.Do(func() {
		tunnel.mu.Lock()
		tunnel.closed = true
		upstream := tunnel.upstream
		cancel := tunnel.cancel
		tunnel.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		_ = tunnel.client.Close()
		if upstream != nil {
			_ = upstream.Close()
		}
	})
}

func (tunnel *responsesWebSocketTunnel) setUpstream(upstream io.Closer) {
	tunnel.mu.Lock()
	closed := tunnel.closed
	if !closed {
		tunnel.upstream = upstream
	}
	tunnel.mu.Unlock()
	if closed && upstream != nil {
		_ = upstream.Close()
	}
}

func (tunnel *responsesWebSocketTunnel) clearUpstream() {
	tunnel.mu.Lock()
	if !tunnel.closed {
		tunnel.upstream = nil
	}
	tunnel.mu.Unlock()
}

func (proxy *Proxy) trackResponsesWebSocketTunnel(tunnel *responsesWebSocketTunnel) func() {
	proxy.mu.Lock()
	if proxy.closing {
		proxy.mu.Unlock()
		tunnel.close()
		return func() {}
	}
	proxy.responsesWebSocketTunnels[tunnel] = struct{}{}
	proxy.mu.Unlock()
	return func() {
		proxy.mu.Lock()
		delete(proxy.responsesWebSocketTunnels, tunnel)
		proxy.mu.Unlock()
	}
}
