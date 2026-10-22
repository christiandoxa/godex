package gemini

import (
	"encoding/json"
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiControlTypesAreCheckedBeforeNormalization(t *testing.T) {
	for _, field := range []string{"stream", "instructions", "previous_response_id"} {
		for _, value := range []any{nil, true, false, "", "opaque", json.Number("7"), []any{}, map[string]any{}} {
			body, err := json.Marshal(map[string]any{"model": "gemini-3.1-pro-preview", "input": "same request", field: value})
			if err != nil {
				t.Fatal(err)
			}
			t.Run(field+"/"+string(body), func(t *testing.T) {
				_, err := normalizeGeminiChatInput043711(body)
				_, valid := value.(string)
				if field == "stream" {
					_, valid = value.(bool)
				}
				if valid {
					if err != nil {
						t.Fatalf("valid control was rejected: %v", err)
					}
					return
				}
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("invalid %s became valid during normalization: %v", field, err)
				}
			})
		}
	}
}
