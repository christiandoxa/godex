package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type websocketTunnel struct {
	client   net.Conn
	upstream io.Closer
	toClient *websocketFrameWriter
	once     sync.Once
	mu       sync.Mutex
	closed   bool
	cancel   context.CancelFunc
}

func (tunnel *websocketTunnel) close() {
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

func (tunnel *websocketTunnel) setUpstream(upstream io.Closer) {
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

func (tunnel *websocketTunnel) clearUpstream() {
	tunnel.mu.Lock()
	if !tunnel.closed {
		tunnel.upstream = nil
	}
	tunnel.mu.Unlock()
}

func (proxy *Proxy) forwardWebSocket(writer http.ResponseWriter, response *proxymodel.Response, lifecycle *requestLifecycle, clientKey string) error {
	upstream, ok := response.Body.(io.ReadWriteCloser)
	if !ok {
		http.Error(writer, "upstream websocket handshake failed", http.StatusBadGateway)
		return io.ErrUnexpectedEOF
	}
	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		http.Error(writer, "websocket upgrades are unavailable", http.StatusBadGateway)
		return http.ErrNotSupported
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		http.Error(writer, "websocket handshake failed", http.StatusBadGateway)
		return err
	}
	tunnel := &websocketTunnel{client: client, upstream: upstream, toClient: &websocketFrameWriter{writer: buffered.Writer}}
	remove := proxy.trackWebSocketTunnel(tunnel)
	defer remove()
	defer tunnel.close()

	lifecycle.commit()
	handshake := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + websocketAcceptKey(clientKey) + "\r\n\r\n"
	if _, err := io.WriteString(buffered.Writer, handshake); err != nil {
		return err
	}
	if err := buffered.Writer.Flush(); err != nil {
		return err
	}

	results := make(chan error, 2)
	go func() {
		results <- copyWebSocketClientFrames(buffered.Reader, upstream, tunnel.toClient)
	}()
	go func() {
		results <- copyWebSocketFrames(upstream, tunnel.toClient)
	}()
	first := <-results
	tunnel.close()
	<-results
	if first == nil {
		lifecycle.complete()
	}
	return first
}

func (proxy *Proxy) trackWebSocketTunnel(tunnel *websocketTunnel) func() {
	// ponytail: active sessions are unbounded; add admission limits if local concurrency needs a ceiling.
	proxy.mu.Lock()
	if proxy.closing {
		proxy.mu.Unlock()
		tunnel.close()
		return func() {}
	}
	proxy.tunnels[tunnel] = struct{}{}
	proxy.mu.Unlock()
	return func() {
		proxy.mu.Lock()
		delete(proxy.tunnels, tunnel)
		proxy.mu.Unlock()
	}
}
