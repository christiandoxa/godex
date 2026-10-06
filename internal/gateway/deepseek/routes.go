package deepseek

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const mountPath = "/backend-api/godex"

type routeKind uint8

const (
	routeResponses routeKind = iota
	routeCompact
	routeChat
	routeMessages
	routeModelsList
	routeModelsSingle
)

type route struct {
	kind    routeKind
	modelID string
}

func runtimeRoute(path string) (route, error) {
	suffix, ok := strings.CutPrefix(path, mountPath)
	if !ok {
		suffix, ok = strings.CutPrefix(path, "/backend-api/prodex")
	}
	if !ok || (suffix != "" && !strings.HasPrefix(suffix, "/")) {
		return route{}, errors.New("DeepSeek runtime received an unsupported proxy path")
	}
	if strings.HasPrefix(suffix, "/v1/") {
		suffix = strings.TrimPrefix(suffix, "/v1")
	}
	switch suffix {
	case "/responses":
		return route{kind: routeResponses}, nil
	case "/responses/compact":
		return route{kind: routeCompact}, nil
	case "/chat/completions":
		return route{kind: routeChat}, nil
	case "/messages":
		return route{kind: routeMessages}, nil
	case "/models":
		return route{kind: routeModelsList}, nil
	}
	if modelID, found := strings.CutPrefix(suffix, "/models/"); found && modelID != "" {
		return route{kind: routeModelsSingle, modelID: modelID}, nil
	}
	return route{}, errors.New("DeepSeek runtime received an unsupported proxy path")
}

func modelsResponse(method string, current route) (*proxymodel.Response, error) {
	if !strings.EqualFold(method, http.MethodGet) {
		return nil, errors.New("DeepSeek model catalog endpoint requires GET")
	}
	models, err := proxymodel.DeepSeekModelsAPI()
	if err != nil {
		return nil, err
	}
	switch current.kind {
	case routeModelsList:
		return jsonResponse(http.StatusOK, map[string]any{"object": "list", "data": models})
	case routeModelsSingle:
		for _, model := range models {
			id, _ := model["id"].(string)
			if strings.EqualFold(id, current.modelID) {
				return jsonResponse(http.StatusOK, model)
			}
		}
		return jsonResponse(http.StatusNotFound, map[string]any{
			"error": map[string]any{
				"message": "model '" + current.modelID + "' is not available for deepseek",
				"type":    "invalid_request_error", "code": "model_not_found",
			},
		})
	default:
		return nil, errors.New("DeepSeek model catalog route is invalid")
	}
}

func jsonResponse(status int, value any) (*proxymodel.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("failed to serialize DeepSeek local response")
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
