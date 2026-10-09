package openai

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/christiandoxa/godex/internal/helper/httpheader"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const DefaultUpstreamURL = "https://chatgpt.com/backend-api"
const upstreamHeaderWait = 30e9
const upstreamIdleTime = 90e9
const websocketConnectTimeout = 15 * time.Second

type authReader interface {
	ReadAuth(context.Context, string) (proxymodel.Auth, error)
}

type unauthorizedAuthRefresher interface {
	RefreshUnauthorizedAuth(context.Context, string, proxymodel.Auth) (proxymodel.Auth, error)
}
type Transport struct {
	client                   *http.Client
	websocketClient          *http.Client
	upstream                 *url.URL
	auth                     authReader
	cookies                  *webSocketCookieJar
	websocketMessageMu       sync.Mutex
	websocketMessageSessions map[uint64]websocketMessageSession
	websocketMessageClosed   bool
}

func NewTransport(upstream string, client *http.Client, auth authReader) (*Transport, error) {
	if upstream == "" {
		upstream = DefaultUpstreamURL
	}
	parsed, err := url.Parse(upstream)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("proxy upstream URL must be an http(s) URL without credentials or query data")
	}
	if auth == nil {
		return nil, errors.New("selected account authentication reader is required")
	}
	client = cloneHTTPClient(client)
	return &Transport{
		client: client, websocketClient: newWebSocketHTTPClient(client), upstream: parsed, auth: auth, cookies: newWebSocketCookieJar(),
		websocketMessageSessions: make(map[uint64]websocketMessageSession),
	}, nil
}
func (transport *Transport) Execute(ctx context.Context, input proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	return transport.execute(ctx, input, account, false)
}

func (transport *Transport) ExecuteWebSocket(ctx context.Context, input proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	return transport.execute(ctx, input, account, true)
}

func (transport *Transport) execute(ctx context.Context, input proxymodel.Request, account proxymodel.Account, websocket bool) (*proxymodel.Response, error) {
	auth, err := transport.auth.ReadAuth(ctx, account.Home)
	if err != nil {
		return nil, err
	}
	response, err := transport.executeWithAuth(ctx, input, account, websocket, auth)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		return response, err
	}

	reloaded, reloadErr := transport.auth.ReadAuth(ctx, account.Home)
	if reloadErr == nil && runtimeAuthChanged(auth, reloaded) {
		_ = response.Body.Close()
		auth = reloaded
		response, err = transport.executeWithAuth(ctx, input, account, websocket, auth)
		if err != nil || response.StatusCode != http.StatusUnauthorized {
			return response, err
		}
	}

	refresher, ok := transport.auth.(unauthorizedAuthRefresher)
	if !ok {
		return response, nil
	}
	refreshed, refreshErr := refresher.RefreshUnauthorizedAuth(ctx, account.Home, auth)
	if refreshErr != nil {
		return response, nil
	}
	_ = response.Body.Close()
	return transport.executeWithAuth(ctx, input, account, websocket, refreshed)
}

func runtimeAuthChanged(previous, current proxymodel.Auth) bool {
	return previous.AccessToken != current.AccessToken || previous.AccountID != current.AccountID
}

func (transport *Transport) executeWithAuth(
	ctx context.Context,
	input proxymodel.Request,
	account proxymodel.Account,
	websocket bool,
	auth proxymodel.Auth,
) (*proxymodel.Response, error) {
	target := *transport.upstream
	target.Path = upstreamPath(target.Path, input.Path)
	if input.RawPath != "" {
		target.RawPath = upstreamPath(transport.upstream.EscapedPath(), input.RawPath)
	}
	target.Path, target.RawPath = protectDotSegments(target.Path, target.RawPath)
	target.RawQuery = input.RawQuery
	request, err := http.NewRequestWithContext(ctx, input.Method, target.String(), bytes.NewReader(input.Body))
	if err != nil {
		return nil, err
	}
	request.Header = input.Header.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	websocketKey := ""
	if websocket {
		websocketKey, err = newWebSocketKey()
		if err != nil {
			return nil, err
		}
		prepareWebSocketRequestHeaders(request.Header, websocketKey)
		if cookie := transport.cookies.header(websocketCookieProfile(account), request.URL, request.Header); cookie != "" {
			request.Header.Set("Cookie", cookie)
		} else {
			request.Header.Del("Cookie")
		}
	} else {
		removeHopHeaders(request.Header)
	}
	request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	if auth.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	var connectCancel context.CancelFunc
	var connectTimer *time.Timer
	if websocket {
		requestContext, cancel, timer := websocketHandshakeContext(ctx, websocketConnectTimeout)
		request = request.WithContext(requestContext)
		connectCancel, connectTimer = cancel, timer
	}
	client := transport.client
	if websocket {
		client = transport.websocketClient
	}
	if websocket && client.Jar != nil {
		copy := *client
		copy.Jar = nil
		client = &copy
	}
	response, err := client.Do(request)
	if err != nil {
		if websocket {
			if !connectTimer.Stop() {
				connectCancel()
				return nil, context.DeadlineExceeded
			}
			connectCancel()
		}
		return nil, err
	}
	if websocket {
		// Stop the handshake deadline after headers; the upgraded body owns cancellation.
		if !connectTimer.Stop() {
			_ = response.Body.Close()
			connectCancel()
			return nil, context.DeadlineExceeded
		}
		transport.cookies.capture(websocketCookieProfile(account), request.URL, response.Header)
	}
	if websocket && response.StatusCode == http.StatusSwitchingProtocols {
		if !strings.EqualFold(strings.TrimSpace(response.Header.Get("Upgrade")), "websocket") ||
			!httpheader.ConnectionTokens(response.Header)[http.CanonicalHeaderKey("Upgrade")] ||
			!validWebSocketAccept(websocketKey, response.Header.Get("Sec-WebSocket-Accept")) {
			_ = response.Body.Close()
			connectCancel()
			return nil, errors.New("upstream websocket handshake response is invalid")
		}
		duplex, ok := response.Body.(io.ReadWriteCloser)
		if !ok {
			_ = response.Body.Close()
			connectCancel()
			return nil, errors.New("upstream websocket connection is not duplex")
		}
		response.Body = &websocketHandshakeBody{ReadCloser: response.Body, duplex: duplex, cancel: connectCancel}
	} else if websocket {
		response.Body = &websocketHandshakeBody{ReadCloser: response.Body, cancel: connectCancel}
	}
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: response.Body, Trailer: response.Trailer}, nil
}

func newWebSocketKey() (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", errors.New("generate upstream websocket key")
	}
	return base64.StdEncoding.EncodeToString(nonce[:]), nil
}

func validWebSocketAccept(key, accept string) bool {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(decoded) != 16 {
		return false
	}
	digest := sha1.Sum([]byte(strings.TrimSpace(key) + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return strings.TrimSpace(accept) == base64.StdEncoding.EncodeToString(digest[:])
}
func (transport *Transport) Close() {
	transport.closeWebSocketMessageSessions()
	transport.client.CloseIdleConnections()
	transport.websocketClient.CloseIdleConnections()
	transport.cookies.clear()
}

func newWebSocketHTTPClient(client *http.Client) *http.Client {
	copy := *client
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		return &copy
	}
	transport = transport.Clone()
	if transport.ResponseHeaderTimeout == 0 || transport.ResponseHeaderTimeout > websocketConnectTimeout {
		transport.ResponseHeaderTimeout = websocketConnectTimeout
	}
	copy.Transport = transport
	return &copy
}

func cloneHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	if copy.Transport == nil {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			copy.Transport = transport.Clone()
		} else {
			copy.Transport = http.DefaultTransport
		}
	}
	if transport, ok := copy.Transport.(*http.Transport); ok {
		transport = transport.Clone()
		transport.DisableCompression = true
		if transport.ResponseHeaderTimeout == 0 {
			transport.ResponseHeaderTimeout = upstreamHeaderWait
		}
		if transport.IdleConnTimeout == 0 {
			transport.IdleConnTimeout = upstreamIdleTime
		}
		copy.Transport = transport
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}
