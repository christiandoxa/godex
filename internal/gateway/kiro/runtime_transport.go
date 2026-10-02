package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const kiroMethodNotAllowedCode = "method_not_allowed"

type RuntimeTransport struct {
	source      *Source
	home        string
	profileName string
	catalog     []map[string]any
	requestID   atomic.Uint64
}

func (source *Source) NewRuntimeTransport(ctx context.Context, home, profileName string) (*RuntimeTransport, error) {
	if source == nil {
		return nil, errors.New("Kiro runtime source is not configured")
	}
	if _, err := source.prepareRuntimeCredential(ctx, home); err != nil {
		return nil, err
	}
	catalog, err := source.runtimeCatalog(home)
	if err != nil {
		return nil, err
	}
	return &RuntimeTransport{source: source, home: home, profileName: profileName, catalog: catalog}, nil
}

func (transport *RuntimeTransport) Execute(ctx context.Context, input proxymodel.Request, _ proxymodel.Account) (*proxymodel.Response, error) {
	route, err := kiroRuntimeRoute(input.Path)
	if err != nil {
		return kiroInvalidRequest(http.StatusNotFound, err.Error(), "unsupported_path")
	}
	if route.kind == routeModelsList || route.kind == routeModelsSingle {
		return transport.modelsResponse(input.Method, route)
	}
	if route.kind == routeCompact {
		if !strings.EqualFold(input.Method, http.MethodPost) {
			return kiroInvalidRequest(http.StatusMethodNotAllowed, "Kiro compact endpoint requires POST", kiroMethodNotAllowedCode)
		}
		return transport.source.semanticCompact(ctx, transport.home, input.Body)
	}
	if !strings.EqualFold(input.Method, http.MethodPost) {
		return kiroInvalidRequest(http.StatusMethodNotAllowed, "Kiro runtime endpoint requires POST", kiroMethodNotAllowedCode)
	}
	request, err := transport.parseRequest(route, input.Body)
	if err != nil {
		return kiroInvalidRequest(http.StatusBadRequest, err.Error(), kiroRequestErrorCode(err))
	}
	request.model = transport.resolveModel(request.model)
	request = applyKiroConversationHistory(transport.source.conversations, transport.profileName, request)
	id := transport.requestID.Add(1) - 1
	if request.stream && (route.kind == routeResponses || route.kind == routeChat) && transport.source.acp == nil {
		body, err := transport.source.liveACPStream(
			ctx, transport.home, request, route, id, transport.profileName,
		)
		if err != nil {
			return nil, err
		}
		return kiroLiveStreamResponse(body), nil
	}
	turn, err := transport.source.executeACPTurn(ctx, transport.home, request.model, request.effort, request.prompt)
	if err != nil {
		return nil, err
	}
	response := kiroResponseFromTurn(turn, id, request.model, transport.profileName)
	rememberKiroConversation(transport.source.conversations, transport.profileName, request, response)
	if request.stream {
		return kiroStreamResponse(route, response, id, request.model)
	}
	return kiroBufferedResponse(route, response, id, request.model)
}

func (transport *RuntimeTransport) parseRequest(route runtimeRoute, body []byte) (runtimeRequest, error) {
	switch route.kind {
	case routeResponses:
		return parseKiroResponsesRequest(body, false)
	case routeChat:
		return parseKiroChatRequest(body)
	case routeMessages:
		return parseKiroMessagesRequest(body)
	default:
		return runtimeRequest{}, errors.New("Kiro runtime route does not accept a request body")
	}
}

func (transport *RuntimeTransport) resolveModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	entries, err := proxymodel.KiroProviderCatalog()
	if err == nil {
		if entry := proxymodel.ResolveProviderCatalogEntry(entries, model); entry != nil {
			return entry.ID
		}
	}
	for _, candidate := range transport.catalog {
		id, _ := candidate["id"].(string)
		if strings.EqualFold(id, model) {
			return id
		}
	}
	return model
}

func (transport *RuntimeTransport) modelsResponse(method string, route runtimeRoute) (*proxymodel.Response, error) {
	if !strings.EqualFold(method, http.MethodGet) {
		return kiroInvalidRequest(http.StatusMethodNotAllowed, "Kiro model catalog endpoint requires GET", kiroMethodNotAllowedCode)
	}
	if route.kind == routeModelsList {
		return kiroJSONResponse(http.StatusOK, map[string]any{"object": "list", "data": transport.catalog})
	}
	for _, model := range transport.catalog {
		id, _ := model["id"].(string)
		if strings.EqualFold(id, route.modelID) {
			return kiroJSONResponse(http.StatusOK, model)
		}
	}
	return kiroInvalidRequest(http.StatusNotFound, "model '"+route.modelID+"' is not available for kiro", "model_not_found")
}

func kiroBufferedResponse(route runtimeRoute, response map[string]any, requestID uint64, requestedModel string) (*proxymodel.Response, error) {
	var body any = response
	switch route.kind {
	case routeChat:
		body = kiroChatResponse(response, requestID)
	case routeMessages:
		body = kiroMessagesResponse(response, requestedModel)
	}
	return kiroJSONResponse(http.StatusOK, body)
}

func kiroLiveStreamResponse(body io.ReadCloser) *proxymodel.Response {
	header := make(http.Header)
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	return &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       body,
		Trailer:    make(http.Header),
	}
}

func kiroJSONResponse(status int, value any) (*proxymodel.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("serialize Kiro runtime response")
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	return &proxymodel.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Trailer: make(http.Header)}, nil
}

func kiroInvalidRequest(status int, message, code string) (*proxymodel.Response, error) {
	return kiroJSONResponse(status, map[string]any{"error": map[string]any{
		"message": message, "type": "invalid_request_error", "code": code,
	}})
}

func (transport *RuntimeTransport) Close() {}

func (transport *RuntimeTransport) String() string {
	return fmt.Sprintf("kiro-runtime(%s)", transport.profileName)
}
