package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	contentTypeHeader = "Content-Type"
	defaultAPIURL     = "https://api.deepseek.com"
	anthropicVersion  = "2023-06-01"
	bodyMaxBytes      = 8 << 20
)

type RuntimeTransport struct {
	client   *http.Client
	upstream *url.URL
	apiKey   string
	options  RequestOptions
}

func NewRuntimeTransport(apiURL, apiKey string, client *http.Client) (*RuntimeTransport, error) {
	return NewRuntimeTransportWithOptions(apiURL, apiKey, RequestOptions{}, client)
}

func NewRuntimeTransportWithOptions(apiURL, apiKey string, options RequestOptions, client *http.Client) (*RuntimeTransport, error) {
	if strings.TrimSpace(apiURL) == "" {
		apiURL = defaultAPIURL
	}
	parsed, err := validateRuntimeURL(apiURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("DeepSeek API credential is unavailable")
	}
	return &RuntimeTransport{client: cloneClient(client), upstream: parsed, apiKey: apiKey}, nil
}

func (transport *RuntimeTransport) Execute(ctx context.Context, input proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	current, err := runtimeRoute(input.Path)
	if err != nil {
		return nil, err
	}
	switch current.kind {
	case routeModelsList, routeModelsSingle:
		return modelsResponse(input.Method, current)
	case routeCompact:
		return compactgateway.LocalFallback(input.Body, "deepseek", "local-policy")
	case routeResponses:
		return transport.executeResponses(ctx, input, current)
	default:
		return transport.executePassthrough(ctx, input, current)
	}
}

func (transport *RuntimeTransport) executeResponses(ctx context.Context, input proxymodel.Request, current route) (*proxymodel.Response, error) {
	model := requestModel(input.Body)
	models := providerentity.ModelFallbackChain("deepseek", model)
	if len(models) == 0 {
		models = []string{"deepseek-v4-pro", "deepseek-v4-flash"}
	}
	for index, candidate := range models {
		body, err := ResponsesRequest(input.Body, RequestOptions{Model: candidate, StrictTools: transport.options.StrictTools})
		if err != nil {
			return nil, &proxymodel.Error{StatusCode: http.StatusBadRequest, Message: err.Error()}
		}
		response, err := transport.send(ctx, input, current, body)
		if err != nil {
			return nil, err
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return translateResponse(response)
		}
		buffered, err := bufferError(response)
		if err != nil {
			return nil, err
		}
		classification := providerentity.ClassifyError(buffered.StatusCode, buffered.body)
		if index+1 < len(models) && providerentity.RetryableAcrossModels(classification.Class) {
			continue
		}
		return buffered.proxyResponse(), nil
	}
	return nil, errors.New("DeepSeek runtime model fallback produced no attempts")
}

func (transport *RuntimeTransport) executePassthrough(ctx context.Context, input proxymodel.Request, current route) (*proxymodel.Response, error) {
	response, err := transport.send(ctx, input, current, input.Body)
	if err != nil {
		return nil, err
	}
	return proxyResponse(response), nil
}

func (transport *RuntimeTransport) send(ctx context.Context, input proxymodel.Request, current route, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, input.Method, transport.target(current, input.RawQuery), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create DeepSeek upstream request")
	}
	applyHeaders(request.Header, input.Header, transport.apiKey, current.kind == routeMessages)
	return transport.client.Do(request)
}

func (transport *RuntimeTransport) target(current route, rawQuery string) string {
	target := *transport.upstream
	base := strings.TrimRight(target.Path, "/")
	switch current.kind {
	case routeResponses, routeChat:
		target.Path = base + "/chat/completions"
	case routeMessages:
		target.Path = deepSeekMessagesPath(base)
	}
	target.RawPath = ""
	target.RawQuery = rawQuery
	return target.String()
}

func deepSeekMessagesPath(basePath string) string {
	basePath = strings.TrimRight(basePath, "/")
	if strings.HasSuffix(basePath, "/anthropic/v1") {
		return basePath + "/messages"
	}
	if strings.HasSuffix(basePath, "/anthropic") {
		return basePath + "/v1/messages"
	}
	for _, suffix := range []string{"/v1", "/beta"} {
		if strings.HasSuffix(basePath, suffix) {
			basePath = strings.TrimSuffix(basePath, suffix)
			break
		}
	}
	return basePath + "/anthropic/v1/messages"
}

func applyHeaders(destination, source http.Header, apiKey string, nativeMessages bool) {
	destination.Set(contentTypeHeader, "application/json")
	destination.Set("Accept-Encoding", "identity")
	destination.Set("Accept", "text/event-stream, application/json")
	if nativeMessages {
		destination.Set("x-api-key", apiKey)
		destination.Set("anthropic-version", anthropicVersion)
	} else {
		destination.Set("Authorization", "Bearer "+apiKey)
	}
	if userAgent := source.Get("User-Agent"); userAgent != "" {
		destination.Set("User-Agent", userAgent)
	}
	for _, name := range []string{"traceparent", "tracestate", "baggage"} {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
}

func translateResponse(response *http.Response) (*proxymodel.Response, error) {
	contentType := strings.ToLower(response.Header.Get(contentTypeHeader))
	if strings.Contains(contentType, "text/event-stream") {
		header := translatedHeaders(response.Header, "text/event-stream")
		return &proxymodel.Response{StatusCode: response.StatusCode, Header: header, Body: chatcompat.ChatSSE(response.Body), Trailer: response.Trailer}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read DeepSeek translated response")
	}
	if len(body) > bodyMaxBytes {
		return nil, errors.New("DeepSeek translated response exceeded the safe read limit")
	}
	translated, err := chatcompat.ChatResponse(body, time.Now())
	if err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     translatedHeaders(response.Header, "application/json"),
		Body:       io.NopCloser(bytes.NewReader(translated)),
		Trailer:    response.Trailer.Clone(),
	}, nil
}

func translatedHeaders(source http.Header, contentType string) http.Header {
	header := source.Clone()
	header.Del("Content-Length")
	header.Del("Content-Encoding")
	header.Set(contentTypeHeader, contentType)
	return header
}

func proxyResponse(response *http.Response) *proxymodel.Response {
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: response.Body, Trailer: response.Trailer}
}

type bufferedResponse struct {
	StatusCode int
	Header     http.Header
	Trailer    http.Header
	body       []byte
}

func bufferError(response *http.Response) (bufferedResponse, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, bodyMaxBytes+1))
	if err != nil {
		return bufferedResponse{}, errors.New("failed to read DeepSeek error response before fallback")
	}
	if len(body) > bodyMaxBytes {
		return bufferedResponse{}, errors.New("DeepSeek error response exceeded the safe read limit")
	}
	return bufferedResponse{StatusCode: response.StatusCode, Header: response.Header.Clone(), Trailer: response.Trailer.Clone(), body: body}, nil
}

func (response bufferedResponse) proxyResponse() *proxymodel.Response {
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: io.NopCloser(bytes.NewReader(response.body)), Trailer: response.Trailer}
}

func requestModel(body []byte) string {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return ""
	}
	model, _ := object["model"].(string)
	return strings.TrimSpace(model)
}

func validateRuntimeURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("DeepSeek runtime API URL must be an http(s) URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("DeepSeek runtime API URL must use http or https")
	}
	return parsed, nil
}

func cloneClient(client *http.Client) *http.Client {
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
