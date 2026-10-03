package gemini

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

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	defaultAPIURL = "https://generativelanguage.googleapis.com/v1beta"
	bodyMaxBytes  = 8 << 20
)

type RuntimeTransport struct {
	client   *http.Client
	upstream *url.URL
	apiKey   string
}

func NewRuntimeTransport(apiURL, apiKey string, client *http.Client) (*RuntimeTransport, error) {
	if strings.TrimSpace(apiURL) == "" {
		apiURL = defaultAPIURL
	}
	parsed, err := validateRuntimeURL(apiURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Gemini runtime API key is unavailable")
	}
	return &RuntimeTransport{client: cloneClient(client), upstream: parsed, apiKey: apiKey}, nil
}

func (transport *RuntimeTransport) Execute(ctx context.Context, input proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	current, err := runtimeRoute(input.Path)
	if err != nil {
		return nil, err
	}
	if (current.kind == routeModels || current.kind == routeModel) && strings.EqualFold(input.Method, http.MethodGet) {
		return modelsResponse(current)
	}
	switch current.kind {
	case routeCompact:
		return transport.executeCompact(ctx, input)
	case routeResponses:
		return transport.executeResponses(ctx, input, current)
	default:
		response, err := transport.send(ctx, input, current, input.Body)
		if err != nil {
			return nil, err
		}
		return proxyResponse(response), nil
	}
}

func (transport *RuntimeTransport) send(ctx context.Context, input proxymodel.Request, current route, body []byte) (*http.Response, error) {
	target, err := transport.target(current, input.RawQuery)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create Gemini upstream request")
	}
	if current.kind == routeMessages || current.kind == routeEmbeddings {
		applyNativeHeaders(request.Header, input.Header, transport.apiKey)
	} else {
		applyHeaders(request.Header, input.Header, transport.apiKey)
	}
	return transport.client.Do(request)
}

func (transport *RuntimeTransport) target(current route, rawQuery string) (string, error) {
	target := *transport.upstream
	base := strings.TrimRight(target.Path, "/")
	switch current.kind {
	case routeResponses, routeChat:
		target.Path = openAICompatiblePath(base, "/chat/completions")
	case routeModels:
		target.Path = openAICompatiblePath(base, "/models")
	case routeModel:
		target.Path = openAICompatiblePath(base, "/models/"+current.modelID)
	case routeMessages:
		target.Path = base + current.upstreamPath
	case routeEmbeddings:
		target.Path = base + current.upstreamPath
	default:
		return "", errors.New("Gemini runtime route has no OpenAI-compatible upstream path")
	}
	target.RawPath = ""
	target.RawQuery = rawQuery
	return target.String(), nil
}

func openAICompatiblePath(base, suffix string) string {
	if strings.HasSuffix(base, "/openai") {
		return base + suffix
	}
	return base + "/openai" + suffix
}

func applyHeaders(destination, source http.Header, apiKey string) {
	destination.Set("Content-Type", "application/json")
	destination.Set("Accept-Encoding", "identity")
	destination.Set("Accept", "text/event-stream, application/json")
	destination.Set("Authorization", "Bearer "+apiKey)
	if value := source.Get("User-Agent"); value != "" {
		destination.Set("User-Agent", value)
	}
	for _, name := range []string{"traceparent", "tracestate", "baggage"} {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
}

func applyNativeHeaders(destination, source http.Header, apiKey string) {
	applyHeaders(destination, source, apiKey)
	destination.Del("Authorization")
	destination.Set("x-goog-api-key", apiKey)
}

func proxyResponse(response *http.Response) *proxymodel.Response {
	return &proxymodel.Response{StatusCode: response.StatusCode, Header: response.Header, Body: response.Body, Trailer: response.Trailer}
}

func validateRuntimeURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Gemini runtime API URL must be an http(s) URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Gemini runtime API URL must use http or https")
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

func jsonResponse(status int, value any) (*proxymodel.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("failed to serialize Gemini local response")
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	return &proxymodel.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Trailer: make(http.Header)}, nil
}
