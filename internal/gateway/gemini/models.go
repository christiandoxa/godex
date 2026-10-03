package gemini

import (
	"errors"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func modelsResponse(current route) (*proxymodel.Response, error) {
	models, err := proxymodel.GeminiModelsAPI()
	if err != nil {
		return nil, err
	}
	if current.kind == routeModels {
		return jsonResponse(http.StatusOK, map[string]any{"object": "list", "data": models})
	}
	if current.kind != routeModel {
		return nil, errors.New("Gemini model catalog route is invalid")
	}
	modelID := strings.TrimSpace(current.modelID)
	for _, model := range models {
		id, _ := model["id"].(string)
		if equalModelID(id, modelID) {
			return jsonResponse(http.StatusOK, model)
		}
		aliases, _ := model["aliases"].([]string)
		for _, alias := range aliases {
			if equalModelID(alias, modelID) {
				return jsonResponse(http.StatusOK, model)
			}
		}
	}
	return jsonResponse(http.StatusNotFound, map[string]any{
		"error": map[string]any{
			"message": "model '" + current.modelID + "' is not available for gemini",
			"type":    "invalid_request_error",
			"code":    "model_not_found",
		},
	})
}

func equalModelID(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range len(left) {
		leftByte, rightByte := left[index], right[index]
		if leftByte >= 'A' && leftByte <= 'Z' {
			leftByte += 'a' - 'A'
		}
		if rightByte >= 'A' && rightByte <= 'Z' {
			rightByte += 'a' - 'A'
		}
		if leftByte != rightByte {
			return false
		}
	}
	return true
}
