package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	providerentity "github.com/christiandoxa/godex/internal/entity/provider"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const modelFallbackResponseMaxBytes = 8 << 20

func (router *Router) executeGeminiModelFallback(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (*proxymodel.Response, error) {
	models, fields, compact, ok := geminiModelFallbacks(request)
	if !ok {
		response, err := router.observedGatewayAttempt(ctx, request, account, func() (*proxymodel.Response, error) {
			return router.gateway.Execute(ctx, request, account)
		})
		if err != nil {
			closeResponse(response)
			return nil, err
		}
		if response == nil {
			return nil, errors.New("Gemini runtime returned no response")
		}
		if !compact || response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			if compact {
				return router.restoreGeminiCompactFallback04384(request.Body, response)
			}
			return response, nil
		}
		closeResponse(response)
		return router.localGeminiCompactFallback(request.Body, "provider-error")
	}
	for index, model := range models {
		attempt := request
		body, err := withModel(fields, model)
		if err != nil {
			return nil, errors.New("failed to encode Gemini model fallback request")
		}
		attempt.Body = body
		response, err := router.observedGatewayAttempt(ctx, attempt, account, func() (*proxymodel.Response, error) {
			return router.gateway.Execute(ctx, attempt, account)
		})
		if err != nil {
			closeResponse(response)
			return nil, err
		}
		if response == nil {
			return nil, errors.New("Gemini runtime returned no response")
		}
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			if compact {
				return router.restoreGeminiCompactFallback04384(request.Body, response)
			}
			return response, nil
		}
		if !compact && response.StatusCode < http.StatusBadRequest {
			return response, nil
		}
		if response.StatusCode >= http.StatusBadRequest && index+1 < len(models) {
			retry, err := retryGeminiModelResponse(response)
			if err != nil {
				if compact {
					return router.localGeminiCompactFallback(request.Body, "provider-error")
				}
				return nil, err
			}
			if retry {
				closeResponse(response)
				continue
			}
		}
		if compact {
			closeResponse(response)
			return router.localGeminiCompactFallback(request.Body, "provider-error")
		}
		return response, nil
	}
	return nil, errors.New("Gemini model fallback produced no attempts")
}

func geminiModelFallbacks(request proxymodel.Request) ([]string, map[string]json.RawMessage, bool, bool) {
	compact := strings.HasSuffix(request.Path, "/responses/compact")
	if !compact && !strings.HasSuffix(request.Path, "/responses") {
		return nil, nil, false, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(request.Body, &fields) != nil || fields == nil {
		return nil, nil, compact, false
	}
	model := "chat-compression-default"
	if !compact {
		if jsonutil.CheckSerdeBoundary(string(request.Body)) != nil || !jsonutil.CheckSerdeNumberTokens(request.Body) {
			return nil, nil, false, false
		}
		model = ""
		if raw, exists := fields["model"]; exists && json.Unmarshal(raw, &model) != nil {
			// The tagged selector treats non-string models as absent.
			model = ""
		}
	}
	return providerentity.ModelFallbackChain("gemini", model), fields, compact, true
}

func (router *Router) restoreGeminiCompactFallback04384(body []byte, response *proxymodel.Response) (*proxymodel.Response, error) {
	if response.Header.Get("X-Prodex-Compact-Mode") != "local-fallback" {
		return response, nil
	}
	reason := response.Header.Get("X-Prodex-Compact-Reason")
	closeResponse(response)
	return router.localGeminiCompactFallback(body, reason)
}

func (router *Router) localGeminiCompactFallback(body []byte, reason string) (*proxymodel.Response, error) {
	fallback, ok := router.gateway.(interface {
		LocalCompactFallback([]byte, string) (*proxymodel.Response, error)
	})
	if !ok {
		return nil, errors.New("Gemini compact fallback is unavailable")
	}
	return fallback.LocalCompactFallback(body, reason)
}

func withModel(fields map[string]json.RawMessage, model string) ([]byte, error) {
	updated := make(map[string]json.RawMessage, len(fields)+1)
	for key, value := range fields {
		updated[key] = value
	}
	encoded, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	updated["model"] = encoded
	body, err := json.Marshal(updated)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func retryGeminiModelResponse(response *proxymodel.Response) (bool, error) {
	if failure := response.PrecommitFailure; failure != nil && !failure.Transport &&
		!providerentity.RetryableAcrossModels(providerentity.ClassifyProviderCode(failure.Code).Class) {
		return false, nil
	}
	if response.Body == nil {
		classification := providerentity.ClassifyError(response.StatusCode, nil)
		return providerentity.RetryableAcrossModels(classification.Class), nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, modelFallbackResponseMaxBytes+1))
	if err != nil {
		closeResponse(response)
		return false, errors.New("failed to read Gemini error response before model fallback")
	}
	if len(body) > modelFallbackResponseMaxBytes {
		response.Body = &prefixReadCloser{Reader: io.MultiReader(bytes.NewReader(body), response.Body), Closer: response.Body}
		return false, nil
	}
	closeResponse(response)
	response.Body = io.NopCloser(bytes.NewReader(body))
	if response.StatusCode == http.StatusTooManyRequests && !providerentity.IsStructuredGemini429(body) {
		return false, nil
	}
	classification := providerentity.ClassifyError(response.StatusCode, body)
	return providerentity.RetryableAcrossModels(classification.Class), nil
}

type prefixReadCloser struct {
	io.Reader
	io.Closer
}

func closeResponse(response *proxymodel.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}
