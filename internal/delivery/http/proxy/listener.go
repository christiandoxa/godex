package proxy

import (
	"net"
	"net/http"
)

func serveProxyListener(server *http.Server, listener net.Listener) error {
	return server.Serve(retryingListener{Listener: listener})
}

// The Rust listener retries every accept error; normalize non-temporary errors
// so net/http keeps its bounded retry and still observes Shutdown cancellation.
type retryingListener struct{ net.Listener }

func (listener retryingListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err == nil {
		return connection, nil
	}
	if networkError, ok := err.(net.Error); ok && networkError.Temporary() {
		return nil, err
	}
	return nil, retryableAcceptError{err: err}
}

type retryableAcceptError struct{ err error }

func (err retryableAcceptError) Error() string { return err.err.Error() }
func (err retryableAcceptError) Unwrap() error { return err.err }
func (retryableAcceptError) Temporary() bool   { return true }
func (err retryableAcceptError) Timeout() bool {
	networkError, ok := err.err.(net.Error)
	return ok && networkError.Timeout()
}
