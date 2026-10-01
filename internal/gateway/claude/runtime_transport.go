package claude

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	"github.com/christiandoxa/godex/internal/helper/httpheader"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	anthropicDefaultAPIURL = "https://api.anthropic.com/v1"
	anthropicAPIVersion    = "2023-06-01"
	anthropicOAuthBeta     = "oauth-2025-04-20"
	anthropicBodyMaxBytes  = 8 << 20
)

type RuntimeTransport struct {
	client   *http.Client
	upstream *url.URL
	auth     RuntimeOAuth
}

func (source *Source) NewRuntimeTransport(ctx context.Context, home, apiURL string, client *http.Client) (*RuntimeTransport, error) {
	auth, err := source.RuntimeOAuth(ctx, home)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiURL) == "" {
		apiURL = anthropicDefaultAPIURL
	}
	return newRuntimeTransport(apiURL, auth, client)
}

func newRuntimeTransport(apiURL string, auth RuntimeOAuth, client *http.Client) (*RuntimeTransport, error) {
	parsed, err := validateAnthropicRuntimeURL(apiURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(auth.accessToken) == "" {
		return nil, errors.New("Anthropic runtime OAuth credential is unavailable")
	}
	return &RuntimeTransport{client: cloneAnthropicClient(client), upstream: parsed, auth: auth}, nil
}

func (transport *RuntimeTransport) Execute(ctx context.Context, input proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	route, err := anthropicRuntimeRoute(input.Path)
	if err != nil {
		return nil, err
	}
	if route.kind == routeModelsList || route.kind == routeModelsSingle {
		return anthropicModelsResponse(input.Method, route)
	}
	if route.kind == routeCompact {
		return compactgateway.LocalFallback(input.Body, "anthropic", "local-policy")
	}
	if route.kind == routeCompact {
		return compactgateway.LocalFallback(input.Body, "anthropic", "local-policy")
	}
	body := input.Body
	translated := route.kind == routeResponses
	if translated {
		body, err = chatcompat.ResponsesRequest(body, "claude-sonnet-4-6", "")
		if err != nil {
			return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
		}
		return transport.executeTranslatedResponses(ctx, input, route, body)
	}
	return transport.executeUpstream(ctx, input, route, body)
}

func (transport *RuntimeTransport) executeTranslatedResponses(
	ctx context.Context,
	input proxymodel.Request,
	route runtimeRoute,
	body []byte,
) (*proxymodel.Response, error) {
	models := anthropicModelFallbackChain(body)
	if len(models) == 0 {
		models = []string{"claude-sonnet-4-6"}
	}
	for index, model := range models {
		response, err := transport.executeUpstream(ctx, input, route, anthropicRequestBodyWithModel(body, model))
		if err != nil {
			return nil, err
		}
		if response.StatusCode < http.StatusBadRequest {
			return translateAnthropicProxyResponse(response)
		}
		buffered, err := bufferAnthropicErrorResponse(response)
		if err != nil {
			return nil, err
		}
		classification := providerentity.ClassifyError(buffered.StatusCode, buffered.body)
		if index+1 < len(models) && providerentity.RetryableAcrossModels(classification.Class) {
			continue
		}
		return buffered.proxyResponse(), nil
	}
	return nil, errors.New("Anthropic runtime model fallback produced no attempts")
}

func (transport *RuntimeTransport) executeUpstream(
	ctx context.Context,
	input proxymodel.Request,
	route runtimeRoute,
	body []byte,
) (*proxymodel.Response, error) {
	target := transport.target(route, input.RawQuery)
	request, err := http.NewRequestWithContext(ctx, input.Method, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create Anthropic upstream request")
	}
	applyAnthropicHeaders(request.Header, input.Header, transport.auth.accessToken, route.kind == routeMessages)
	response, err := transport.client.Do(request)
	if err != nil {
		return nil, err
	}
	return anthropicProxyResponse(response), nil
}

func (transport *RuntimeTransport) target(route runtimeRoute, rawQuery string) string {
	target := *transport.upstream
	suffix := "/chat/completions"
	if route.kind == routeMessages {
		suffix = "/messages"
	}
	target.Path = strings.TrimRight(target.Path, "/") + suffix
	target.RawPath = ""
	target.RawQuery = rawQuery
	return target.String()
}

func applyAnthropicHeaders(destination, source http.Header, accessToken string, nativeMessages bool) {
	copyAnthropicRequestHeaders(destination, source)
	destination.Set("Authorization", "Bearer "+accessToken)
	destination.Del("ChatGPT-Account-Id")
	destination.Set("anthropic-beta", anthropicOAuthBeta)
	destination.Set("Content-Type", "application/json")
	destination.Set("Accept-Encoding", "identity")
	destination.Set("Accept", "text/event-stream, application/json")
	if nativeMessages {
		destination.Set("anthropic-version", anthropicAPIVersion)
	} else {
		destination.Del("anthropic-version")
	}
}

func copyAnthropicRequestHeaders(destination, source http.Header) {
	connectionHeaders := httpheader.ConnectionTokens(source)
	for key, values := range source {
		name := strings.ToLower(strings.TrimSpace(key))
		if httpheader.IsRequestTransport(name) || connectionHeaders[http.CanonicalHeaderKey(key)] ||
			strings.HasPrefix(name, "sec-websocket-") || strings.HasPrefix(name, "x-prodex-internal-") ||
			name == "authorization" || name == "chatgpt-account-id" {
			continue
		}
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func translateAnthropicProxyResponse(response *proxymodel.Response) (*proxymodel.Response, error) {
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		header := translatedAnthropicHeaders(response.Header, "text/event-stream")
		return &proxymodel.Response{StatusCode: response.StatusCode, Header: header, Body: chatcompat.ChatSSE(response.Body), Trailer: response.Trailer}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, anthropicBodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read Anthropic translated response")
	}
	if len(body) > anthropicBodyMaxBytes {
		return nil, errors.New("Anthropic translated response exceeded the safe read limit")
	}
	translated, err := chatcompat.ChatResponse(body, time.Now())
	if err != nil {
		return nil, err
	}
	header := translatedAnthropicHeaders(response.Header, "application/json")
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: header, Body: io.NopCloser(bytes.NewReader(translated)), Trailer: response.Trailer.Clone()}, nil
}

type bufferedAnthropicResponse struct {
	StatusCode int
	Header     http.Header
	Trailer    http.Header
	body       []byte
}

func bufferAnthropicErrorResponse(response *proxymodel.Response) (bufferedAnthropicResponse, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, anthropicBodyMaxBytes+1))
	if err != nil {
		return bufferedAnthropicResponse{}, errors.New("failed to read Anthropic error response before fallback")
	}
	if len(body) > anthropicBodyMaxBytes {
		return bufferedAnthropicResponse{}, errors.New("Anthropic error response exceeded the safe read limit")
	}
	return bufferedAnthropicResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Trailer:    response.Trailer.Clone(),
		body:       body,
	}, nil
}

func (response bufferedAnthropicResponse) proxyResponse() *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     response.Header,
		Body:       io.NopCloser(bytes.NewReader(response.body)),
		Trailer:    response.Trailer,
	}
}

func translatedAnthropicHeaders(source http.Header, contentType string) http.Header {
	header := source.Clone()
	header.Del("Content-Length")
	header.Del("Content-Encoding")
	header.Set("Content-Type", contentType)
	return header
}

func anthropicProxyResponse(response *http.Response) *proxymodel.Response {
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: response.Body, Trailer: response.Trailer}
}

func validateAnthropicRuntimeURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Anthropic runtime API URL must be an http(s) URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Anthropic runtime API URL must use http or https")
	}
	return parsed, nil
}

func cloneAnthropicClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	copy := *client
	if copy.Transport == nil {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			copy.Transport = transport.Clone()
		}
	}
	if transport, ok := copy.Transport.(*http.Transport); ok {
		transport = transport.Clone()
		transport.DisableCompression = true
		if transport.ResponseHeaderTimeout == 0 {
			transport.ResponseHeaderTimeout = 30 * time.Second
		}
		if transport.IdleConnTimeout == 0 {
			transport.IdleConnTimeout = 90 * time.Second
		}
		copy.Transport = transport
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}

func (transport *RuntimeTransport) Close() { transport.client.CloseIdleConnections() }
