package routing

import (
	"encoding/json"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// The original Responses request's top-level stream:true is authoritative
// only when the upstream omits a MIME type. Explicit JSON remains unary,
// even for a streaming request, as in Prodex 0.437.1.
func requestedResponsesStream(request proxymodel.Request) bool {
	if request.WebSocketMessage || strings.EqualFold(strings.TrimSpace(request.Header.Get("Upgrade")), "websocket") ||
		(request.QuotaSelection.RouteKind != quotamodel.RouteKindResponses &&
			!strings.HasSuffix(strings.TrimSuffix(request.Path, "/"), "/responses")) {
		return false
	}
	var values struct {
		Stream *bool `json:"stream"`
	}
	return json.Unmarshal(request.Body, &values) == nil && values.Stream != nil && *values.Stream
}
