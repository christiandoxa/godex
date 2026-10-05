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
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) persistTransportBackoff(ctx context.Context, accountID string, selection quotamodel.Selection) {
	route := routeHealthRoute(selection.RouteKind)
	if ctx.Err() != nil || accountID == "" || route == "" {
		return
	}
	router.transportMu.Lock()
	defer router.transportMu.Unlock()

	now := router.now()
	key := routeHealthKey{accountID: accountID, route: route}
	router.mu.Lock()
	current, exists := router.transportBackoffs[key]
	router.mu.Unlock()
	seconds := int64(routingentity.InitialTransportBackoffDuration / time.Second)
	if exists && current.UntilUnix > now.Unix() {
		remaining := current.UntilUnix - now.Unix()
		seconds = min(max(remaining*2, seconds), int64(routingentity.MaxTransportBackoffDuration/time.Second))
	}
	backoff := routingentity.TransportBackoff{AccountID: accountID, Route: route, UntilUnix: now.Unix() + seconds}
	router.mu.Lock()
	if router.transportBackoffs == nil {
		router.transportBackoffs = make(map[routeHealthKey]routingentity.TransportBackoff)
	}
	if current, exists := router.transportBackoffs[key]; exists && current.UntilUnix > backoff.UntilUnix {
		backoff = current
	}
	router.transportBackoffs[key] = backoff
	router.mu.Unlock()
	if router.state != nil {
		_ = router.state.SetTransportBackoff(ctx, backoff, now)
	}
}

func (router *Router) clearTransportBackoff(ctx context.Context, accountID string, selection quotamodel.Selection) {
	route := routeHealthRoute(selection.RouteKind)
	if ctx.Err() != nil || accountID == "" || route == "" {
		return
	}
	router.transportMu.Lock()
	defer router.transportMu.Unlock()
	router.mu.Lock()
	delete(router.transportBackoffs, routeHealthKey{accountID: accountID, route: route})
	router.mu.Unlock()
	if router.state != nil {
		_ = router.state.ClearTransportBackoff(ctx, accountID, route)
	}
}

func (router *Router) transportBackoffRemaining(accountID string, selection quotamodel.Selection, now time.Time) time.Duration {
	route := routeHealthRoute(selection.RouteKind)
	if accountID == "" || route == "" {
		return 0
	}
	key := routeHealthKey{accountID: accountID, route: route}
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
			if transportFailureMessage(cause.Error()) {
				return true
			}
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
	return false
}

func transportFailureMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	for _, marker := range []string{
		"dns",
		"failed to lookup address information",
		"no such host",
		"name or service not known",
		"connection refused",
		"timed out",
		"timeout",
		"tls",
		"handshake",
		"certificate",
		"connection reset",
		"broken pipe",
		"unexpected eof",
		"connection aborted",
		"connection closed before message completed",
		"stream closed before response.completed",
		"closed before response.completed",
		"unable to connect",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
