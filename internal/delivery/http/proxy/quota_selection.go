package proxy

import (
	"encoding/json"
	"strings"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const maxQuotaModelBytes = 256

func quotaSelection(path string, websocket bool, body []byte) quotamodel.Selection {
	selection := quotamodel.Selection{}
	switch {
	case websocket:
		selection.RouteKind = quotamodel.RouteKindWebSocket
	case strings.HasSuffix(strings.TrimRight(path, "/"), "/responses/compact"):
		selection.RouteKind = quotamodel.RouteKindCompact
	case strings.HasSuffix(strings.TrimRight(path, "/"), "/responses"):
		selection.RouteKind = quotamodel.RouteKindResponses
	}
	selection.RequestedModel = requestModel(body)
	return selection
}

func requestModel(body []byte) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return ""
	}
	model := rawString(object["model"])
	if model == "" {
		var response map[string]json.RawMessage
		if json.Unmarshal(object["response"], &response) == nil {
			model = rawString(response["model"])
		}
	}
	model = strings.TrimSpace(model)
	if len(model) > maxQuotaModelBytes {
		return ""
	}
	return model
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}
