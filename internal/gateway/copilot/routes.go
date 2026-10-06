package copilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type copilotRouteKind uint8

const (
	copilotRouteUpstream copilotRouteKind = iota
	copilotRouteModelsList
	copilotRouteModelsSingle
)

type copilotRoute struct {
	kind         copilotRouteKind
	upstreamPath string
	modelID      string
}

func copilotRuntimeRoute(path string) (copilotRoute, error) {
	suffix, err := copilotMountedSuffix(path)
	if err != nil {
		return copilotRoute{}, err
	}
	switch suffix {
	case "/responses", "/responses/compact", "/chat/completions", "/messages":
		return copilotRoute{kind: copilotRouteUpstream, upstreamPath: suffix}, nil
	case "/models":
		return copilotRoute{kind: copilotRouteModelsList}, nil
	}
	if modelID, ok := strings.CutPrefix(suffix, "/models/"); ok && modelID != "" {
		return copilotRoute{kind: copilotRouteModelsSingle, modelID: modelID}, nil
	}
	return copilotRoute{}, errors.New("Copilot runtime received an unsupported proxy path")
}

func copilotMountedSuffix(path string) (string, error) {
	suffix, ok := strings.CutPrefix(path, copilotMountPath)
	if !ok {
		suffix, ok = strings.CutPrefix(path, "/backend-api/prodex")
	}
	if !ok || (suffix != "" && !strings.HasPrefix(suffix, "/")) {
		return "", errors.New("Copilot runtime received an unsupported proxy path")
	}
	if strings.HasPrefix(suffix, "/v1/") {
		suffix = strings.TrimPrefix(suffix, "/v1")
	}
	return suffix, nil
}

func (transport *RuntimeTransport) modelsResponse(method string, route copilotRoute) (*proxymodel.Response, error) {
	if !strings.EqualFold(method, http.MethodGet) {
		return nil, errors.New("Copilot model catalog endpoint requires GET")
	}
	models := transport.ModelCatalog()
	switch route.kind {
	case copilotRouteModelsList:
		return copilotJSONResponse(http.StatusOK, map[string]any{"object": "list", "data": models})
	case copilotRouteModelsSingle:
		for _, model := range models {
			id := runtimeCatalogEntryID(model)
			if id != "" && strings.EqualFold(id, route.modelID) {
				return copilotJSONResponse(http.StatusOK, model)
			}
		}
		return copilotJSONResponse(http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"message": "model '" + route.modelID + "' is not available for copilot",
				"type":    "invalid_request_error",
				"code":    "model_not_found",
			},
		})
	default:
		return nil, errors.New("Copilot model catalog route is invalid")
	}
}

func copilotJSONResponse(status int, value any) (*proxymodel.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("failed to serialize Copilot local response")
	}
	header := make(http.Header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	return &proxymodel.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Trailer:    make(http.Header),
	}, nil
}
