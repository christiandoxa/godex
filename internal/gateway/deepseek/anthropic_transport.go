package deepseek

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func translateAnthropicResponse(response *http.Response, requestMetadata map[string]any) (*proxymodel.Response, error) {
	return translateAnthropicResponseWithRequestID(response, requestMetadata, 0)
}

func translateAnthropicResponseWithRequestID(response *http.Response, requestMetadata map[string]any, requestID uint64) (*proxymodel.Response, error) {
	contentType := strings.ToLower(response.Header.Get(contentTypeHeader))
	if strings.Contains(contentType, "text/event-stream") {
		return &proxymodel.Response{
			StatusCode: response.StatusCode,
			Header:     translatedHeaders(response.Header, "text/event-stream; charset=utf-8"),
			Body:       deepSeekAnthropicSSEWithRequestID(response.Body, requestMetadata, requestID), Trailer: response.Trailer,
		}, nil
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, nativeMessagesMaxBytes+1))
	if err != nil {
		return nil, errors.New("failed to read native DeepSeek Messages response")
	}
	if len(body) > nativeMessagesMaxBytes {
		return nil, errors.New("native DeepSeek Messages response exceeded the safe read limit")
	}
	translated, err := deepSeekAnthropicResponse(body, time.Now())
	if err != nil {
		return nil, err
	}
	translated, err = mergeAnthropicResponseMetadata(translated, requestMetadata)
	if err != nil {
		return nil, err
	}
	return translatedDeepSeekResponseWithContentType(response, translated, nil, "application/json; charset=utf-8")
}
