package gemini

import (
	"errors"
	"strings"
)

const mountPath = "/backend-api/godex"

type routeKind uint8

const (
	routeResponses routeKind = iota
	routeCompact
	routeChat
	routeMessages
	routeEmbeddings
	routeModels
	routeModel
)

type route struct {
	kind         routeKind
	modelID      string
	upstreamPath string
}

func runtimeRoute(path string) (route, error) {
	suffix, ok := strings.CutPrefix(path, mountPath)
	if !ok {
		suffix, ok = strings.CutPrefix(path, "/backend-api/prodex")
	}
	if !ok || (suffix != "" && !strings.HasPrefix(suffix, "/")) {
		return route{}, errors.New("Gemini runtime received an unsupported proxy path")
	}
	upstreamPath := suffix
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
		return route{kind: routeMessages, upstreamPath: upstreamPath}, nil
	case "/embeddings":
		return route{kind: routeEmbeddings, upstreamPath: upstreamPath}, nil
	case "/models":
		return route{kind: routeModels}, nil
	}
	if modelID, found := strings.CutPrefix(suffix, "/models/"); found && validModelIDPath(modelID) {
		return route{kind: routeModel, modelID: modelID}, nil
	}
	return route{}, errors.New("Gemini runtime received an unsupported proxy path")
}

func validModelIDPath(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.Contains(id, "/")
}
