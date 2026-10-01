package copilot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
	return &RuntimeTransport{client: cloneRuntimeClient(client), upstream: parsed, auth: auth}, nil
}

func (transport *RuntimeTransport) ModelCatalog() []map[string]any {
	return transport.auth.ModelCatalog()
}

func (transport *RuntimeTransport) Execute(ctx context.Context, input proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	path, err := copilotUpstreamPath(input.Path)
	if err != nil {
		return nil, err
	}
	target := *transport.upstream
	target.Path = strings.TrimRight(target.Path, "/") + path
	target.RawPath = ""
	target.RawQuery = input.RawQuery
	body := canonicalizeCopilotRequest(input.Body)
	request, err := http.NewRequestWithContext(ctx, input.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("create Copilot upstream request")
	}
	applyCopilotHeaders(request.Header, body, transport.auth.apiKey)
	response, err := transport.client.Do(request)
	if err != nil {
		return nil, err
	}
	return &proxymodel.Response{
		StatusCode: response.StatusCode,
		Header:     response.Header,
		Body:       response.Body,
		Trailer:    response.Trailer,
	}, nil
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

func copilotRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "godex-runtime"
	}
	return "godex-" + hex.EncodeToString(value[:])
}

func copilotUpstreamPath(path string) (string, error) {
	suffix, ok := strings.CutPrefix(path, copilotMountPath)
	if !ok || (suffix != "" && !strings.HasPrefix(suffix, "/")) {
		return "", errors.New("Copilot runtime received an unsupported proxy path")
	}
	if legacy, rest, ok := splitLegacyRuntimeVersion(suffix); ok && legacy {
		suffix = rest
	}
	if suffix == "/responses" || suffix == "/responses/compact" {
		return suffix, nil
	}
	return "", errors.New("Copilot runtime currently supports Responses endpoints only")
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
