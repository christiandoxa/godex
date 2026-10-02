package routing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type modelFallbackResult struct {
	status int
	body   string
}

type modelFallbackGateway struct {
	results        []modelFallbackResult
	models         []string
	fallbackReason string
	fallbackBody   string
}

func (gateway *modelFallbackGateway) Execute(
	_ context.Context,
	request proxymodel.Request,
	_ proxymodel.Account,
) (*proxymodel.Response, error) {
	var body struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(request.Body, &body); err != nil {
		return nil, err
	}
	gateway.models = append(gateway.models, body.Model)
	result := gateway.results[len(gateway.models)-1]
	return &proxymodel.Response{
		StatusCode: result.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(result.body)),
	}, nil
}

func (gateway *modelFallbackGateway) LocalCompactFallback(body []byte, reason string) (*proxymodel.Response, error) {
	gateway.fallbackReason = reason
	gateway.fallbackBody = string(body)
	return &proxymodel.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("local fallback"))}, nil
}

func TestGeminiModelFallbackRetriesOnlyBeforeResponseCommit(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		results    []modelFallbackResult
		wantModels []string
		wantBody   string
		fallback   bool
	}{
		{
			name:       "unsupported model uses next candidate",
			path:       "/backend-api/prodex/v1/responses",
			results:    []modelFallbackResult{{http.StatusNotFound, `{"error":{"code":"model_not_supported"}}`}, {http.StatusOK, `{"ok":true}`}},
			wantModels: []string{"gemini-3.1-pro-preview", "gemini-3.8-flash"},
			wantBody:   `{"ok":true}`,
		},
		{
			name:       "unstructured rate limit is preserved",
			path:       "/backend-api/prodex/v1/responses",
			results:    []modelFallbackResult{{http.StatusTooManyRequests, "too many requests\n"}},
			wantModels: []string{"gemini-3.1-pro-preview"},
			wantBody:   "too many requests\n",
		},
		{
			name:       "429 model error stays terminal",
			path:       "/backend-api/prodex/v1/responses",
			results:    []modelFallbackResult{{http.StatusTooManyRequests, `{"error":{"code":"model_not_supported"}}`}},
			wantModels: []string{"gemini-3.1-pro-preview"},
			wantBody:   `{"error":{"code":"model_not_supported"}}`,
		},
		{
			name:       "structured rate limit uses next candidate",
			path:       "/backend-api/prodex/v1/responses",
			results:    []modelFallbackResult{{http.StatusTooManyRequests, `{"error":{"code":"rate_limit_exceeded"}}`}, {http.StatusOK, `{"ok":true}`}},
			wantModels: []string{"gemini-3.1-pro-preview", "gemini-3.8-flash"},
			wantBody:   `{"ok":true}`,
		},
		{
			name:       "daily quota detail uses next candidate",
			path:       "/backend-api/prodex/v1/responses",
			results:    []modelFallbackResult{{http.StatusTooManyRequests, `{"error":{"details":[{"quotaId":"GenerateContentPerDay"}]}}`}, {http.StatusOK, `{"ok":true}`}},
			wantModels: []string{"gemini-3.1-pro-preview", "gemini-3.8-flash"},
			wantBody:   `{"ok":true}`,
		},
		{
			name:       "oversized error stays streaming",
			path:       "/backend-api/prodex/v1/responses",
			results:    []modelFallbackResult{{http.StatusServiceUnavailable, strings.Repeat("x", modelFallbackResponseMaxBytes+16)}},
			wantModels: []string{"gemini-3.1-pro-preview"},
			wantBody:   strings.Repeat("x", modelFallbackResponseMaxBytes+16),
		},
		{
			name: "compact ends with local fallback",
			path: "/backend-api/prodex/v1/responses/compact",
			results: []modelFallbackResult{
				{http.StatusNotFound, `{"error":{"code":"model_not_supported"}}`},
				{http.StatusNotFound, `{"error":{"code":"model_not_supported"}}`},
				{http.StatusNotFound, `{"error":{"code":"model_not_supported"}}`},
				{http.StatusNotFound, `{"error":{"code":"model_not_supported"}}`},
			},
			wantModels: []string{"gemini-3.8-flash", "gemini-3.5-flash", "gemini-3-flash-preview", "gemini-2.5-flash"},
			wantBody:   "local fallback",
			fallback:   true,
		},
	}
	for _, fixture := range cases {
		t.Run(fixture.name, func(t *testing.T) {
			gateway := &modelFallbackGateway{results: fixture.results}
			router := &Router{gateway: gateway}
			response, err := router.execute(context.Background(), proxymodel.Request{
				Path: fixture.path,
				Body: []byte(`{"model":"auto","input":"hello"}`),
			}, proxymodel.Account{Provider: proxymodel.Provider{Kind: "gemini"}})
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(gateway.models, ",") != strings.Join(fixture.wantModels, ",") {
				t.Fatalf("models = %q, want %q", gateway.models, fixture.wantModels)
			}
			if string(body) != fixture.wantBody {
				t.Fatalf("response body length=%d, want %d", len(body), len(fixture.wantBody))
			}
			if fixture.fallback && (gateway.fallbackReason != "upstream-error" || gateway.fallbackBody != `{"model":"auto","input":"hello"}`) {
				t.Fatalf("compact fallback reason=%q body=%q", gateway.fallbackReason, gateway.fallbackBody)
			}
		})
	}
}
