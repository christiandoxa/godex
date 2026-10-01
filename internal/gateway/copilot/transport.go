package copilot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	copilotRuntimeUserAgent = "copilot/1.0.65 (client/github/cli)"
	copilotMountPath        = "/backend-api/prodex"
)

type RuntimeTransport struct {
	client   *http.Client
	upstream *url.URL
	auth     RuntimeAuth
}

func NewRuntimeTransport(upstream string, auth RuntimeAuth, client *http.Client) (*RuntimeTransport, error) {
	parsed, err := validateRuntimeUpstream(upstream)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(auth.apiKey) == "" {
		return nil, errors.New("Copilot runtime credential is unavailable")
	}
	staticCatalog, err := proxymodel.CopilotProviderCatalogJSON()
	if err != nil {
		return nil, err
	}
	auth.modelCatalog, err = mergeRuntimeCatalog(staticCatalog, auth.modelCatalog)
	if err != nil {
		return nil, err
	}
	return &RuntimeTransport{client: cloneRuntimeClient(client), upstream: parsed, auth: auth}, nil
}

func (transport *RuntimeTransport) ModelCatalog() []map[string]any {
	return transport.auth.ModelCatalog()
}

func (transport *RuntimeTransport) Execute(ctx context.Context, input proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	route, err := copilotRuntimeRoute(input.Path)
	if err != nil {
		return nil, err
	}
	if route.kind != copilotRouteUpstream {
		return transport.modelsResponse(input.Method, route)
	}
	target := *transport.upstream
	target.Path = strings.TrimRight(target.Path, "/") + route.upstreamPath
	target.RawPath = ""
	target.RawQuery = input.RawQuery
	models := copilotModelFallbackChain(input.Body)
	for index, model := range models {
		response, err := transport.executeModel(ctx, input.Method, target.String(), input.Header, input.Body, model)
		if err != nil {
			return nil, err
		}
		if response.StatusCode < 400 || index+1 >= len(models) {
			return proxyResponse(response), nil
		}
		buffered, err := bufferCopilotErrorResponse(response)
		if err != nil {
			return nil, err
		}
		if copilotModelRetryAllowed(buffered.StatusCode, buffered.body) {
			continue
		}
		return buffered.proxyResponse(), nil
	}
	return nil, errors.New("Copilot runtime model fallback produced no attempts")
}

func (transport *RuntimeTransport) executeModel(
	ctx context.Context,
	method, target string,
	sourceHeader http.Header,
	originalBody []byte,
	model string,
) (*http.Response, error) {
	body := copilotRequestBodyWithModel(originalBody, model)
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create Copilot upstream request")
	}
	copyCopilotTraceHeaders(request.Header, sourceHeader)
	applyCopilotHeaders(request.Header, body, transport.auth.apiKey)
	return transport.client.Do(request)
}

func proxyResponse(response *http.Response) *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     response.Header,
		Body:       response.Body,
		Trailer:    response.Trailer,
	}
}

type bufferedCopilotResponse struct {
	StatusCode int
	Header     http.Header
	Trailer    http.Header
	body       []byte
}

func bufferCopilotErrorResponse(response *http.Response) (bufferedCopilotResponse, error) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, runtimeBodyMaxBytes+1))
	if err != nil {
		return bufferedCopilotResponse{}, errors.New("failed to read Copilot error response before fallback")
	}
	if len(body) > runtimeBodyMaxBytes {
		return bufferedCopilotResponse{}, errors.New("Copilot error response exceeded the safe read limit")
	}
	return bufferedCopilotResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Trailer:    response.Trailer.Clone(),
		body:       body,
	}, nil
}

func (response bufferedCopilotResponse) proxyResponse() *proxymodel.Response {
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     response.Header,
		Body:       io.NopCloser(bytes.NewReader(response.body)),
		Trailer:    response.Trailer,
	}
}

func (transport *RuntimeTransport) Close() { transport.client.CloseIdleConnections() }

func applyCopilotHeaders(header http.Header, body []byte, apiKey string) {
	header.Set("Authorization", "Bearer "+apiKey)
	header.Set("Content-Type", "application/json")
	header.Set("Accept-Encoding", "identity")
	header.Set("Accept", "text/event-stream, application/json")
	header.Set("Copilot-Integration-Id", runtimeIntegrationID)
	header.Set("Openai-Intent", "conversation-panel")
	header.Set("X-GitHub-Api-Version", runtimeAPIVersion)
	header.Set("X-Request-Id", copilotRequestID())
	header.Set("User-Agent", copilotRuntimeUserAgent)
	if copilotHasAgentInput(body) {
		header.Set("X-Initiator", "agent")
	} else {
		header.Set("X-Initiator", "user")
	}
	if copilotHasVisionInput(body) {
		header.Set("Copilot-Vision-Request", "true")
	}
}

func copyCopilotTraceHeaders(destination, source http.Header) {
	for _, name := range []string{"traceparent", "tracestate", "baggage"} {
		if value := source.Get(name); value != "" {
			destination.Set(name, value)
		}
	}
}

func copilotRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "godex-runtime"
	}
	return "godex-" + hex.EncodeToString(value[:])
}

func splitLegacyRuntimeVersion(suffix string) (bool, string, bool) {
	if !strings.HasPrefix(suffix, "/v") {
		return false, suffix, false
	}
	rest := strings.TrimPrefix(suffix, "/v")
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return false, suffix, false
	}
	version := rest[:slash]
	digit := false
	for _, current := range version {
		if current >= '0' && current <= '9' {
			digit = true
			continue
		}
		if current != '.' {
			return false, suffix, false
		}
	}
	if !digit {
		return false, suffix, false
	}
	return true, rest[slash:], true
}

func validateRuntimeUpstream(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Copilot runtime API URL must be an http(s) URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("Copilot runtime API URL must use http or https")
	}
	return parsed, nil
}

func cloneRuntimeClient(client *http.Client) *http.Client {
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
