package kiro

import (
	"errors"
	"strings"
)

const runtimeMountPath = "/backend-api/prodex"

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

func kiroRuntimeRoute(path string) (runtimeRoute, error) {
	suffix, ok := strings.CutPrefix(path, runtimeMountPath)
	if !ok || (suffix != "" && !strings.HasPrefix(suffix, "/")) {
		return runtimeRoute{}, errors.New("Kiro runtime received an unsupported proxy path")
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
	if modelID, found := strings.CutPrefix(suffix, "/models/"); found && modelID != "" && !strings.Contains(modelID, "/") {
		return runtimeRoute{kind: routeModelsSingle, modelID: modelID}, nil
	}
	return runtimeRoute{}, errors.New("Kiro runtime received an unsupported proxy path")
}
