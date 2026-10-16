package proxy

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type scriptedListener04380 struct {
	accepts chan struct{}
	closed  chan struct{}
	calls   atomic.Int32
	once    sync.Once
}

func newScriptedListener04380() *scriptedListener04380 {
	return &scriptedListener04380{accepts: make(chan struct{}, 2), closed: make(chan struct{})}
}

func (listener *scriptedListener04380) Accept() (net.Conn, error) {
	listener.accepts <- struct{}{}
	if listener.calls.Add(1) == 1 {
		return nil, errors.New("synthetic permanent accept failure")
	}
	<-listener.closed
	return nil, net.ErrClosed
}

func (*scriptedListener04380) Addr() net.Addr { return scriptedAddr04380{} }

func (listener *scriptedListener04380) Close() error {
	listener.once.Do(func() { close(listener.closed) })
	return nil
}

type scriptedAddr04380 struct{}

func (scriptedAddr04380) Network() string { return "tcp" }
func (scriptedAddr04380) String() string  { return "scripted" }

func TestProxyListenerRetriesPermanentAcceptFailureUntilShutdown(t *testing.T) {
	listener := newScriptedListener04380()
	server := &http.Server{}
	t.Cleanup(func() { _ = server.Close() })
	done := make(chan error, 1)
	go func() { done <- serveProxyListener(server, listener) }()
	select {
	case <-listener.accepts:
	case <-time.After(time.Second):
		t.Fatal("listener was not called")
	}
	select {
	case <-listener.accepts:
	case err := <-done:
		t.Fatalf("server stopped after permanent accept failure: %v", err)
	case <-time.After(time.Second):
		t.Fatal("listener was not retried")
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serve result = %v, want server closed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after shutdown")
	}
}
