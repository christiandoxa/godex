package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const anthropicMountPath = "/backend-api/godex"

type runtimeRouteKind uint8

const (
	routeResponses runtimeRouteKind = iota
	routeCompact
	routeChat
	routeMessages
	routeModelsList
	routeModelsSingle
)

type runtimeRoute struct {
	kind    runtimeRouteKind
	modelID string
}

func anthropicRuntimeRoute(path string) (runtimeRoute, error) {
	suffix, ok := strings.CutPrefix(path, anthropicMountPath)
	if !ok {
		suffix, ok = strings.CutPrefix(path, "/backend-api/prodex")
	}
	if !ok || (suffix != "" && !strings.HasPrefix(suffix, "/")) {
		return runtimeRoute{}, errors.New("Anthropic runtime received an unsupported proxy path")
	}
	if strings.HasPrefix(suffix, "/v1/") {
		suffix = strings.TrimPrefix(suffix, "/v1")
	}
	switch suffix {
	case "/responses":
		return runtimeRoute{kind: routeResponses}, nil
	case "/responses/compact":
		return runtimeRoute{kind: routeCompact}, nil
	case "/chat/completions":
		return runtimeRoute{kind: routeChat}, nil
	case "/messages":
		return runtimeRoute{kind: routeMessages}, nil
	case "/models":
		return runtimeRoute{kind: routeModelsList}, nil
	}
	if modelID, found := strings.CutPrefix(suffix, "/models/"); found && modelID != "" {
		return runtimeRoute{kind: routeModelsSingle, modelID: modelID}, nil
	}
	return runtimeRoute{}, errors.New("Anthropic runtime received an unsupported proxy path")
}

func anthropicModelsResponse(method string, route runtimeRoute) (*proxymodel.Response, error) {
	if !strings.EqualFold(method, http.MethodGet) {
		return nil, errors.New("Anthropic model catalog endpoint requires GET")
	}
	models, err := proxymodel.AnthropicModelsAPI()
	if err != nil {
		return nil, err
	}
	switch route.kind {
	case routeModelsList:
		return anthropicJSONResponse(http.StatusOK, map[string]any{"object": "list", "data": models})
	case routeModelsSingle:
		for _, model := range models {
			id, _ := model["id"].(string)
			if strings.EqualFold(id, route.modelID) {
				return anthropicJSONResponse(http.StatusOK, model)
			}
		}
		return anthropicJSONResponse(http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"message": "model '" + route.modelID + "' is not available for anthropic",
				"type":    "invalid_request_error", "code": "model_not_found",
			},
		})
	default:
		return nil, errors.New("Anthropic model catalog route is invalid")
	}
}

func anthropicJSONResponse(status int, value any) (*proxymodel.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("failed to serialize Anthropic local response")
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	return &proxymodel.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Trailer: make(http.Header)}, nil
}
