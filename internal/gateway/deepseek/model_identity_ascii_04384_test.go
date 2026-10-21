package deepseek

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// Tagged 0.438.4 prodex_mojo_rich_catalog_resolve_exact_v1 uses
// catalog_byte_equal_folded: only A-Z ASCII folds, never Unicode U+212A
// KELVIN SIGN into k. Mixed/uppercase *ASCII* model queries remain valid.
func TestProdex04384DeepSeekModelCatalogIdentityASCIIOnly(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		status      int
		wantID      string
	}{
		{"canonical", "deepseek-v4-pro", http.StatusOK, "deepseek-v4-pro"},
		{"uppercase-ASCII", "DEEPSEEK-V4-PRO", http.StatusOK, "deepseek-v4-pro"},
		{"mixedcase-ASCII", "DeepSeek-V4-Pro", http.StatusOK, "deepseek-v4-pro"},
		{"kelvin-confusable", "deepseeK-v4-pro", http.StatusNotFound, ""},
		{"kelvin-uppercase", "DEEPSEEK-V4-PRO", http.StatusNotFound, ""},
		{"leading-space", " deepseek-v4-pro", http.StatusNotFound, ""},
		{"trailing-space", "deepseek-v4-pro ", http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := modelsResponse(http.MethodGet, route{kind: routeModelsSingle, modelID: tc.model})
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status {
				t.Fatalf("model query=%q status=%d want=%d", tc.model, response.StatusCode, tc.status)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if err := json.Unmarshal(body, &data); err != nil {
				t.Fatal(err)
			}
			if tc.status == http.StatusOK {
				if data["id"] != tc.wantID {
					t.Fatalf("model query=%q yielded wrong model=%#v", tc.model, data)
				}
			} else if detail, ok := data["error"].(map[string]any); !ok || detail["code"] != "model_not_found" {
				t.Fatalf("model query=%q returned unexpected failure=%#v", tc.model, data)
			}
		})
	}
}
