package proxy

import (
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestQuotaSelectionReadsOpenAIAndWebSocketModels(t *testing.T) {
	for _, test := range []struct {
		name      string
		path      string
		websocket bool
		body      string
		kind      quotamodel.RouteKind
		model     string
	}{
		{"responses", "/backend-api/codex/responses", false, `{"model":"gpt-5.6-luna"}`, quotamodel.RouteKindResponses, "gpt-5.6-luna"},
		{"compact", "/backend-api/codex/responses/compact", false, `{"model":"gpt-5.6-sol"}`, quotamodel.RouteKindCompact, "gpt-5.6-sol"},
		{"websocket", "/backend-api/codex/responses", true, `{"type":"response.create","response":{"model":"luna"}}`, quotamodel.RouteKindWebSocket, "luna"},
		{"standard", "/v1/chat/completions", false, `{"model":"gpt-5.6-sol"}`, quotamodel.RouteKindStandard, "gpt-5.6-sol"},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection := quotaSelection(test.path, test.websocket, []byte(test.body))
			if selection.RouteKind != test.kind || selection.RequestedModel != test.model {
				t.Fatalf("quota selection = %+v", selection)
			}
		})
	}
}

func TestQuotaSelectionBoundsRequestedModel(t *testing.T) {
	selection := quotaSelection("/backend-api/codex/responses", false, []byte(`{"model":"`+strings.Repeat("x", maxQuotaModelBytes+1)+`"}`))
	if selection.RequestedModel != "" {
		t.Fatalf("oversized requested model retained: %d bytes", len(selection.RequestedModel))
	}
}

func TestProdex04377ProxyQuotaSelectionCarriesCodeReviewAdmission(t *testing.T) {
	proxy := &Proxy{broker: &proxymodel.BrokerConfig{IncludeCodeReview: true}}
	selection := proxy.quotaSelection("/backend-api/codex/responses", false, []byte(`{"model":"gpt-5.6-sol"}`))
	if !selection.IncludeCodeReview || selection.RouteKind != quotamodel.RouteKindResponses {
		t.Fatalf("quota selection = %+v", selection)
	}
}
