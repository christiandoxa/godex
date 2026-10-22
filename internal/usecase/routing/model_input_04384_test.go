package routing

import (
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiModelTypesUseDefaultBeforeGateway(t *testing.T) {
	for _, model := range []string{"null", "7", "false", "[]", "{}", `""`, `" \t "`, "18446744073709551615", `"auto"`, `"gemini-3.1-pro-preview"`} {
		t.Run(model, func(t *testing.T) {
			gateway := &modelFallbackGateway{results: []modelFallbackResult{{http.StatusOK, `{"ok":true}`}}}
			router := &Router{gateway: gateway}
			response, err := router.execute(t.Context(), proxymodel.Request{Path: "/backend-api/godex/responses",
				Body: []byte(`{"input":"same request","model":` + model + `}`)},
				proxymodel.Account{Provider: proxymodel.Provider{Kind: "gemini"}})
			if response != nil {
				defer response.Body.Close()
			}
			if err != nil || response == nil || len(gateway.models) != 1 || gateway.models[0] != "gemini-3.1-pro-preview" {
				t.Fatalf("model selection did not use the tagged default once: error=%v models=%v", err, gateway.models)
			}
		})
	}
}

func TestProdex04384GeminiModelRewritePreservesInvalidSerdeInput(t *testing.T) {
	for _, model := range []string{`"\ud800"`, "\"\xff\"", "1e309"} {
		t.Run(model, func(t *testing.T) {
			_, _, _, rewrite := geminiModelFallbacks(proxymodel.Request{Path: "/backend-api/godex/responses",
				Body: []byte(`{"input":"same request","model":` + model + `}`)})
			if rewrite {
				t.Fatal("invalid original serde value was eligible for model repair before gateway admission")
			}
		})
	}
}
