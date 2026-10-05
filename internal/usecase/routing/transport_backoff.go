package routing

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type transportBackoffKey struct {
	accountID string
	route     string
}

func transportRoute(request proxymodel.Request) string {
	if request.WebSocketMessage {
		return "websocket"
	}
	path := strings.ToLower(strings.TrimSpace(request.Path))
	switch {
	case strings.HasSuffix(path, "/responses/compact"):
		return "compact"
	case strings.HasSuffix(path, "/responses"):
		return "responses"
	default:
		return "standard"
	}
}

func (router *Router) persistTransportBackoff(ctx context.Context, accountID string, request proxymodel.Request) {
	if ctx.Err() != nil || accountID == "" {
		return
	}
	key := transportBackoffKey{accountID: accountID, route: transportRoute(request)}
	router.transportBackoffMu.Lock()
	defer router.transportBackoffMu.Unlock()

	now := router.now()
	router.mu.Lock()
	current, exists := router.transportBackoffs[key]
	router.mu.Unlock()
	seconds := int64(15)
	if exists && current.UntilUnix > now.Unix() {
		remaining := current.UntilUnix - now.Unix()
		seconds = min(max(remaining*2, int64(15)), int64(120))
	}
	backoff := routingentity.TransportBackoff{
		AccountID: accountID, Route: key.route, UntilUnix: now.Unix() + seconds,
	}
	router.mu.Lock()
	if current, exists := router.transportBackoffs[key]; exists && current.UntilUnix > backoff.UntilUnix {
		backoff = current
	}
	router.transportBackoffs[key] = backoff
	router.mu.Unlock()
	if router.state != nil {
		_ = router.state.SetTransportBackoff(ctx, backoff, now)
	}
}

func (router *Router) clearTransportBackoff(ctx context.Context, accountID string, request proxymodel.Request) {
	if ctx.Err() != nil || accountID == "" {
		return
	}
	key := transportBackoffKey{accountID: accountID, route: transportRoute(request)}
	router.transportBackoffMu.Lock()
	defer router.transportBackoffMu.Unlock()
	router.mu.Lock()
	delete(router.transportBackoffs, key)
	router.mu.Unlock()
	if router.state != nil {
		_ = router.state.ClearTransportBackoff(ctx, accountID, key.route)
	}
}

func (router *Router) transportBackoffRemaining(accountID string, request proxymodel.Request, now time.Time) time.Duration {
	key := transportBackoffKey{accountID: accountID, route: transportRoute(request)}
	router.mu.Lock()
	defer router.mu.Unlock()
	backoff, exists := router.transportBackoffs[key]
	if !exists {
		return 0
	}
	remaining := backoff.Remaining(now)
	if remaining == 0 {
		delete(router.transportBackoffs, key)
	}
	return remaining
}

func isTransportFailure(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if _, wrappedURL := cause.(*url.Error); wrappedURL {
			continue
		}
		if _, network := cause.(net.Error); network {
			return true
		}
		switch cause.(type) {
		case tls.AlertError, tls.RecordHeaderError, x509.UnknownAuthorityError,
			x509.HostnameError, x509.CertificateInvalidError, *tls.CertificateVerificationError:
			return true
		}
		if transportFailureMessage(cause.Error()) {
			return true
		}
	}
	return transportFailureMessage(err.Error())
}

func transportFailureMessage(message string) bool {
	message = strings.ToLower(message)
	for _, marker := range []string{
		"failed to lookup address information",
		"no such host",
		"tls handshake",
		"handshake timed out",
		"connection reset",
		"connection refused",
		"broken pipe",
		"unexpected eof",
		"stream closed before response.completed",
		"connection closed before message completed",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
