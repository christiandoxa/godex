package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

// The 0.438.4 provider bridge resolves dynamic IDs exactly first, then
// calls prodex-provider-core::provider_model_json for static IDs AND
// aliases (`pro`, `auto`, `flash`). Godex previously searched model IDs only.
func TestProdex04384DeepSeekModelsResolveBuiltInAliases(t *testing.T) {
	for _, tc := range []struct {
		name, alias, want string
		status            int
	}{
		{"pro", "pro", "deepseek-v4-pro", http.StatusOK},
		{"auto", "auto", "deepseek-v4-pro", http.StatusOK},
		{"uppercase-auto", "AUTO", "deepseek-v4-pro", http.StatusOK},
		{"mixedcase-pro", "PrO", "deepseek-v4-pro", http.StatusOK},
		{"flash", "flash", "deepseek-v4-flash", http.StatusOK},
		{"uppercase-flash", "FLASH", "deepseek-v4-flash", http.StatusOK},
		{"unicode-kelvin-model", "deepseeK-v4-pro", "", http.StatusNotFound},
		{"unknown-model", "unknown-synthetic-model", "", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := modelsResponse(http.MethodGet, route{kind: routeModelsSingle, modelID: tc.alias})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("DeepSeek static model alias %q status=%d want=%d", tc.alias, response.StatusCode, tc.status)
			}
			var body map[string]any
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if tc.status == http.StatusOK {
				if body["id"] != tc.want {
					t.Fatalf("DeepSeek alias %q returned %#v want=%s", tc.alias, body, tc.want)
				}
			} else {
				failure, _ := body["error"].(map[string]any)
				if failure["code"] != "model_not_found" {
					t.Fatalf("DeepSeek alias %q wrong missing model error %#v", tc.alias, body)
				}
			}
		})
	}
}

func TestProdex04384DeepSeekAliasLookupNeverUsesProviderCredential(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	transport, err := NewRuntimeTransport(server.URL, "synthetic-credential", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	for _, id := range []string{"pro", "auto", "flash"} {
		result, err := transport.Execute(context.Background(), proxymodel.Request{Method: http.MethodGet, Path: mountPath + "/models/" + id}, proxymodel.Account{})
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(result.Body)
		result.Body.Close()
		if result.StatusCode != http.StatusOK || len(body) == 0 {
			t.Fatalf("DeepSeek alias %q local model failed: %d %s", id, result.StatusCode, body)
		}
	}
	if calls != 0 {
		t.Fatalf("DeepSeek built-in model aliases leaked %d requests to authenticated provider", calls)
	}
}
