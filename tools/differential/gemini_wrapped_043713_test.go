package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Guard the upstream oracle independently from the candidate: the wrapped
// fixture must contain the *native* candidate text/usage under response, not
// a synthetic already-translated Responses or Chat Completion payload.
func TestProdex043713GeminiWrappedDifferentialFixture(t *testing.T) {
	for _, tt := range []struct {
		scenario  string
		wantTrace bool
	}{
		{"wrapped-response-trace", true}, {"wrapped-response-no-trace", false},
	} {
		t.Run(tt.scenario, func(t *testing.T) {
			mock := newGeminiMock(tt.scenario)
			defer mock.server.Close()
			request, err := http.NewRequest(http.MethodPost, mock.server.URL+"/v1beta/openai/chat/completions", strings.NewReader(`{"model":"gemini-3.1-pro-preview","messages":[]}`))
			if err != nil {
				t.Fatal(err)
			}
			response, err := mock.server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := json.Unmarshal(body, &root); err != nil {
				t.Fatal(err)
			}
			envelope, ok := root["response"].(map[string]any)
			if !ok || envelope["responseId"] != "stale-nested-id" || envelope["candidates"] == nil ||
				envelope["modelVersion"] != "gemini-3.1-pro-preview" || envelope["usageMetadata"] == nil {
				t.Fatalf("not a native Gemini response envelope: %s", body)
			}
			trace, present := root["traceId"].(string)
			if present != tt.wantTrace || tt.wantTrace && trace != "synthetic-trace-id" {
				t.Fatalf("trace fixture missing or spoofed: %s", body)
			}
			if _, alreadyTranslated := root["output"]; alreadyTranslated {
				t.Fatal("fixture used translated response rather than native wrapped payload")
			}
		})
	}
}
