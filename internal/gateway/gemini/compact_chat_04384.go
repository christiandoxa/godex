package gemini

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	compactgateway "github.com/christiandoxa/godex/internal/gateway/compact"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (transport *RuntimeTransport) executeChatCompatibleCompact04384(ctx context.Context, input proxymodel.Request) (*proxymodel.Response, error) {
	body, err := semanticCompactRequest(input.Body, requestModel(input.Body))
	if err != nil {
		return compactgateway.LocalFallback(input.Body, "gemini", "invalid-response")
	}
	body, _, err = prepareGeminiChatCompatibleRequest04384(body)
	if err != nil {
		return compactgateway.LocalFallback(input.Body, "gemini", geminiCompactFailureReason04384(err))
	}
	input.RawQuery = ""
	response, err := transport.send(ctx, input, route{kind: routeResponses}, body)
	if err != nil {
		return compactgateway.LocalFallback(input.Body, "gemini", geminiCompactFailureReason04384(err))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return proxyResponse(response), nil
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, bufferedChatResponseMaxBytes+1))
	if err != nil || len(content) > bufferedChatResponseMaxBytes {
		return compactgateway.LocalFallback(input.Body, "gemini", "provider-error")
	}
	summary := compactSummary(content)
	if summary == "" {
		return compactgateway.LocalFallback(input.Body, "gemini", "invalid-response")
	}
	return compactgateway.Semantic(semanticCompactContinuationSummary(summary, input.Body), "gemini")
}

func geminiCompactFailureReason04384(err error) string {
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	var connection *net.OpError
	if errors.As(err, &connection) && connection.Op == "dial" {
		return "unavailable"
	}
	message := strings.ToLower(err.Error())
	for _, group := range []struct {
		reason string
		terms  []string
	}{
		{"timeout", []string{"timed out", "timeout"}},
		{"unavailable", []string{"unavailable", "connection refused", "could not connect", "failed to spawn"}},
		{"unsupported", []string{"unsupported"}},
		{"invalid-response", []string{"parse", "missing", "no summary", "invalid", "unexpectedly returned"}},
	} {
		for _, term := range group.terms {
			if strings.Contains(message, term) {
				return group.reason
			}
		}
	}
	return "provider-error"
}
