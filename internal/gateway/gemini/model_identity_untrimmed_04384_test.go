package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// The tagged runtime_provider_model_json_for uses the Mojo
// rich_catalog_resolve_exact_v1 classifier on the ORIGINAL model ID.
// Leading/trailing spaces cannot silently mint a valid alias or model.
func TestProdex04384GeminiModelCatalogDoesNotTrimQueryIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		status      int
		wantID      string
	}{
		{"canonical", "gemini-3.1-pro-preview", http.StatusOK, "gemini-3.1-pro-preview"},
		{"ascii-uppercase", "GEMINI-3.1-PRO-PREVIEW", http.StatusOK, "gemini-3.1-pro-preview"},
		{"leading-space", " gemini-3.1-pro-preview", http.StatusNotFound, ""},
		{"trailing-space", "gemini-3.1-pro-preview ", http.StatusNotFound, ""},
		{"both-spaces", " gemini-3.1-pro-preview ", http.StatusNotFound, ""},
		{"padded-alias", " auto ", http.StatusNotFound, ""},
		{"canonical-alias", "auto", http.StatusOK, "auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := modelsResponse(route{kind: routeModel, modelID: tc.query})
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tc.status {
				t.Fatalf("Gemini model query %q status=%d want=%d", tc.query, response.StatusCode, tc.status)
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
					t.Fatalf("Gemini model query=%q resolved %#v want id=%q", tc.query, data, tc.wantID)
				}
			} else if detail, ok := data["error"].(map[string]any); !ok || detail["code"] != "model_not_found" {
				t.Fatalf("Gemini model query %q returned unexpected error %#v", tc.query, data)
			}
		})
	}
}
