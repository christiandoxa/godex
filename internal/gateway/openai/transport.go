package openai

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const DefaultUpstreamURL = "https://chatgpt.com/backend-api"
const upstreamHeaderWait = 30e9
const upstreamIdleTime = 90e9

type authReader interface {
	ReadAuth(context.Context, string) (proxymodel.Auth, error)
}
type Transport struct {
	client   *http.Client
	upstream *url.URL
	auth     authReader
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
	return &Transport{client: cloneHTTPClient(client), upstream: parsed, auth: auth}, nil
}
func (transport *Transport) Execute(ctx context.Context, input proxymodel.Request, account proxymodel.Account) (*proxymodel.Response, error) {
	auth, err := transport.auth.ReadAuth(ctx, account.Home)
	if err != nil {
		return nil, err
	}
	target := *transport.upstream
	target.Path = upstreamPath(target.Path, input.Path)
	target.RawPath = upstreamPath(transport.upstream.EscapedPath(), input.RawPath)
	target.RawQuery = input.RawQuery
	request, err := http.NewRequestWithContext(ctx, input.Method, target.String(), bytes.NewReader(input.Body))
	if err != nil {
		return nil, err
	}
	request.Header = input.Header.Clone()
	removeHopHeaders(request.Header)
	request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	if auth.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", auth.AccountID)
	}
	response, err := transport.client.Do(request)
	if err != nil {
		return nil, err
	}
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: response.Body, Trailer: response.Trailer}, nil
}
func (transport *Transport) Close() { transport.client.CloseIdleConnections() }

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
